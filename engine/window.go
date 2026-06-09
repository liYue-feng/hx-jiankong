package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// ========== Windows API ==========
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	pw = user32.NewProc
	_  = gdi32.NewProc
)

var (
	procFindWindowW      = user32.NewProc("FindWindowW")
	procEnumWindows      = user32.NewProc("EnumWindows")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procGetClassNameW    = user32.NewProc("GetClassNameW")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procPrintWindow      = user32.NewProc("PrintWindow")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procBitBlt                 = gdi32.NewProc("BitBlt")
)

type HWND uintptr
type HDC uintptr
type HBITMAP uintptr

type WinRect struct{ Left, Top, Right, Bottom int32 }
type BITMAPINFOHEADER struct {
	BiSize, BiWidth, BiHeight int32
	BiPlanes, BiBitCount      uint16
	BiCompression, BiSizeImage uint32
	BiXPelsPerMeter, BiYPelsPerMeter int32
	BiClrUsed, BiClrImportant uint32
}
type BITMAPINFO struct{ BmiHeader BITMAPINFOHEADER }

const (
	WM_MOUSEMOVE   = 0x0200
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
	WM_KEYDOWN     = 0x0100
	WM_KEYUP       = 0x0101
	WM_CHAR        = 0x0102
	VK_ESCAPE      = 0x1B
	SRCCOPY        = 0x00CC0020
	PW_CLIENTONLY  = 1
)

type WinInfo struct {
	HWND    uintptr
	Title   string
	Class   string
	Visible bool
	W, H    int
}

// getClassName 获取窗口类名
func getClassName(hwnd HWND) string {
	var buf [256]uint16
	procGetClassNameW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 256)
	return syscall.UTF16ToString(buf[:])
}

// isBrowserClass 判断窗口类名是否为浏览器
func isBrowserClass(cls string) bool {
	lower := strings.ToLower(cls)
	browserKeywords := []string{"chrome", "cef", "edge", "webview", "mozilla", "opera", "safari"}
	for _, kw := range browserKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isWeChatClass 判断窗口类名是否为微信
func isWeChatClass(cls string) bool {
	lower := strings.ToLower(cls)
	return strings.Contains(lower, "wechat") || strings.Contains(lower, "wx")
}

// FindWeChatWindow 查找微信主窗口（按标题+类名过滤，不含浏览器）
func FindWeChatWindow() *WinInfo {
	var found *WinInfo
	callback := syscall.NewCallback(func(hwnd HWND, lParam uintptr) uintptr {
		ret, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if ret == 0 {
			return 1
		}

		var buf [256]uint16
		procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 256)
		title := syscall.UTF16ToString(buf[:])
		cls := getClassName(hwnd)

		// 排除浏览器窗口
		if isBrowserClass(cls) {
			return 1
		}

		// 标题包含"微信" 或 类名是微信类
		if strings.Contains(title, "微信") || isWeChatClass(cls) {
			var rect WinRect
			procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
			w, h := int(rect.Right-rect.Left), int(rect.Bottom-rect.Top)
			if w > 100 && h > 100 { // 过滤掉小控件
				found = &WinInfo{
					HWND:    uintptr(hwnd),
					Title:   title,
					Class:   cls,
					W:       w,
					H:       h,
					Visible: true,
				}
				return 0
			}
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

// FindWindowByTitle 查找包含指定标题的窗口（排除浏览器窗口）
func FindWindowByTitle(titleParts ...string) *WinInfo {
	var found *WinInfo
	callback := syscall.NewCallback(func(hwnd HWND, lParam uintptr) uintptr {
		ret, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if ret == 0 {
			return 1
		}

		cls := getClassName(hwnd)
		if isBrowserClass(cls) {
			return 1
		}

		var buf [256]uint16
		procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 256)
		title := syscall.UTF16ToString(buf[:])

		for _, part := range titleParts {
			if part == "" || title == "" {
				continue
			}
			if strings.Contains(title, part) {
				var rect WinRect
				procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
				w, h := int(rect.Right-rect.Left), int(rect.Bottom-rect.Top)
				if w > 100 && h > 100 {
					found = &WinInfo{
						HWND:    uintptr(hwnd),
						Title:   title,
						Class:   cls,
						W:       w,
						H:       h,
						Visible: true,
					}
					return 0
				}
			}
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

// ClickAt 向指定窗口发送鼠标点击
func ClickAt(hwnd uintptr, x, y int) {
	lParam := uintptr((y << 16) | (x & 0xFFFF))
	procPostMessageW.Call(hwnd, WM_MOUSEMOVE, 0, lParam)
	procPostMessageW.Call(hwnd, WM_LBUTTONDOWN, 1, lParam)
	procPostMessageW.Call(hwnd, WM_LBUTTONUP, 0, lParam)
}

// PressKey 向指定窗口发送按键
func PressKey(hwnd uintptr, keyCode int) {
	procPostMessageW.Call(hwnd, WM_KEYDOWN, uintptr(keyCode), 0)
	procPostMessageW.Call(hwnd, WM_KEYUP, uintptr(keyCode), 0)
}

func TypeText(hwnd uintptr, text string) {
	for _, ch := range text {
		procPostMessageW.Call(hwnd, WM_CHAR, uintptr(ch), 0)
	}
}

// CaptureWindow 截图目标窗口
func CaptureWindow(hwnd uintptr) (image.Image, error) {
	dc, _, _ := procGetDC.Call(hwnd)
	if dc == 0 {
		return nil, fmt.Errorf("GetDC 失败")
	}
	defer procReleaseDC.Call(hwnd, dc)

	var rect WinRect
	ret, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	}

	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("窗口尺寸无效: %dx%d", w, h)
	}

	// 方案一: PrintWindow
	img, err := capturePW(hwnd, dc, w, h)
	if err == nil && !isBlankImage(img) {
		return img, nil
	}

	// 方案二: BitBlt
	img, err = captureBitBlt(hwnd, w, h)
	if err == nil && !isBlankImage(img) {
		return img, nil
	}

	return capturePWFull(hwnd, dc, w, h)
}

func capturePW(hwnd uintptr, dc uintptr, w, h int) (image.Image, error) {
	memDC, _, _ := procCreateCompatibleDC.Call(dc)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(memDC)

	bmp, _, _ := procCreateCompatibleBitmap.Call(dc, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap 失败")
	}
	defer procDeleteObject.Call(bmp)

	oldBmp, _, _ := procSelectObject.Call(memDC, bmp)
	defer procSelectObject.Call(memDC, oldBmp)

	ret, _, _ := procPrintWindow.Call(hwnd, memDC, PW_CLIENTONLY)
	if ret == 0 {
		return nil, fmt.Errorf("PrintWindow 失败")
	}

	return bitmapToImage(HBITMAP(bmp), w, h)
}

func captureBitBlt(hwnd uintptr, w, h int) (image.Image, error) {
	var rect WinRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC(NULL) 失败")
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(memDC)

	bmp, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap 失败")
	}
	defer procDeleteObject.Call(bmp)

	oldBmp, _, _ := procSelectObject.Call(memDC, bmp)
	defer procSelectObject.Call(memDC, oldBmp)

	procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, uintptr(rect.Left), uintptr(rect.Top), SRCCOPY)

	return bitmapToImage(HBITMAP(bmp), w, h)
}

func capturePWFull(hwnd uintptr, dc uintptr, w, h int) (image.Image, error) {
	memDC, _, _ := procCreateCompatibleDC.Call(dc)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(memDC)

	bmp, _, _ := procCreateCompatibleBitmap.Call(dc, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap 失败")
	}
	defer procDeleteObject.Call(bmp)

	oldBmp, _, _ := procSelectObject.Call(memDC, bmp)
	defer procSelectObject.Call(memDC, oldBmp)

	ret, _, _ := procPrintWindow.Call(hwnd, memDC, 2) // PW_RENDERFULLCONTENT
	if ret == 0 {
		return nil, fmt.Errorf("PrintWindow(RENDERFULLCONTENT) 失败")
	}

	return bitmapToImage(HBITMAP(bmp), w, h)
}

func bitmapToImage(bmp HBITMAP, w, h int) (image.Image, error) {
	bits := make([]byte, w*h*4)
	bi := BITMAPINFO{
		BmiHeader: BITMAPINFOHEADER{
			BiSize: 40, BiWidth: int32(w), BiHeight: -int32(h),
			BiPlanes: 1, BiBitCount: 32, BiCompression: 0,
		},
	}

	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)

	ret, _, _ := procGetDIBits.Call(dc, uintptr(bmp), 0, uintptr(h),
		uintptr(unsafe.Pointer(&bits[0])), uintptr(unsafe.Pointer(&bi)), 0)
	if int(ret) == 0 {
		return nil, fmt.Errorf("GetDIBits 失败")
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := (y*w + x) * 4
			img.Set(x, y, color.RGBA{R: bits[idx+2], G: bits[idx+1], B: bits[idx], A: bits[idx+3]})
		}
	}
	return img, nil
}

func isBlankImage(img image.Image) bool {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return true
	}
	whiteCount := 0
	sampleStep := max(1, (w*h)/500)
	total := 0
	for y := 0; y < h; y += sampleStep {
		for x := 0; x < w; x += sampleStep {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 > 240 && g>>8 > 240 && b>>8 > 240 {
				whiteCount++
			}
			total++
		}
	}
	return total > 0 && float64(whiteCount)/float64(total) > 0.95
}

// RegionPixels 计算指定区域的非白色像素数
func RegionPixels(img image.Image, left, top, right, bottom int) int {
	bounds := img.Bounds()
	if left < bounds.Min.X {
		left = bounds.Min.X
	}
	if top < bounds.Min.Y {
		top = bounds.Min.Y
	}
	if right > bounds.Max.X {
		right = bounds.Max.X
	}
	if bottom > bounds.Max.Y {
		bottom = bounds.Max.Y
	}

	count := 0
	for y := top; y < bottom; y += 2 {
		for x := left; x < right; x += 2 {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
				count++
			}
		}
	}
	return count
}

// CropImage 裁剪图像区域
func CropImage(img image.Image, left, top, right, bottom int) image.Image {
	bounds := img.Bounds()
	if left < bounds.Min.X {
		left = bounds.Min.X
	}
	if top < bounds.Min.Y {
		top = bounds.Min.Y
	}
	if right > bounds.Max.X {
		right = bounds.Max.X
	}
	if bottom > bounds.Max.Y {
		bottom = bounds.Max.Y
	}
	return img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(image.Rect(left, top, right, bottom))
}

// SaveImage 保存图片到文件
func SaveImage(img image.Image, path string) error {
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// StripSpaces 去除字符串中的空格和换行
func StripSpaces(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "")
	return s
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
