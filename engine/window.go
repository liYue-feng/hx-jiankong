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
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")
)

var (
	procFindWindowW      = user32.NewProc("FindWindowW")
	procEnumWindows      = user32.NewProc("EnumWindows")
	procEnumChildWindows = user32.NewProc("EnumChildWindows")
	procGetWindowTextW   = user32.NewProc("GetWindowTextW")
	procGetClassNameW    = user32.NewProc("GetClassNameW")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procIsWindowVisible  = user32.NewProc("IsWindowVisible")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procPrintWindow      = user32.NewProc("PrintWindow")
	procSendMessageW     = user32.NewProc("SendMessageW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procClientToScreen   = user32.NewProc("ClientToScreen")
	procMapWindowPoints  = user32.NewProc("MapWindowPoints")

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
	BiSize, BiWidth, BiHeight          int32
	BiPlanes, BiBitCount               uint16
	BiCompression, BiSizeImage         uint32
	BiXPelsPerMeter, BiYPelsPerMeter   int32
	BiClrUsed, BiClrImportant          uint32
}
type BITMAPINFO struct{ BmiHeader BITMAPINFOHEADER }

const (
	WM_MOUSEMOVE   = 0x0200
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
	WM_KEYDOWN     = 0x0100
	WM_KEYUP       = 0x0101
	WM_CHAR        = 0x0102
	WM_SETCURSOR   = 0x0020
	MK_LBUTTON     = 0x0001
	VK_ESCAPE      = 0x1B
	VK_BACK        = 0x08
	SRCCOPY        = 0x00CC0020
	PW_CLIENTONLY  = 1
)

type WinInfo struct {
	HWND       uintptr
	Title      string
	Class      string
	Visible    bool
	ScreenX    int
	ScreenY    int
	ClientW    int
	ClientH    int
	W, H       int
	ChildHWND  uintptr // 小程序内容区的子窗口句柄
}

// getClassName 获取窗口类名
func getClassName(hwnd HWND) string {
	var buf [256]uint16
	procGetClassNameW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 256)
	return syscall.UTF16ToString(buf[:])
}

func isBrowserClass(cls string) bool {
	lower := strings.ToLower(cls)
	for _, kw := range []string{"chrome", "cef", "edge", "webview", "mozilla", "opera", "safari"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isWeChatClass(cls string) bool {
	lower := strings.ToLower(cls)
	return strings.Contains(lower, "wechat") || strings.Contains(lower, "wx")
}

// FindWeChatWindow 查找微信主窗口，同时枚举子窗口找到小程序内容区
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

		if isBrowserClass(cls) {
			return 1
		}

		if strings.Contains(title, "微信") || isWeChatClass(cls) {
			var rect WinRect
			procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
			w, h := int(rect.Right-rect.Left), int(rect.Bottom-rect.Top)
			if w > 100 && h > 100 {
				// 获取客户区尺寸
				var clientRect WinRect
				procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&clientRect)))

				info := &WinInfo{
					HWND:    uintptr(hwnd),
					Title:   title,
					Class:   cls,
					ScreenX: int(rect.Left),
					ScreenY: int(rect.Top),
					ClientW: int(clientRect.Right),
					ClientH: int(clientRect.Bottom),
					W:       w,
					H:       h,
					Visible: true,
				}

				// 枚举子窗口，找到小程序内容区
				// 微信小程序内容在子窗口中渲染，类名通常含 "WebKit" / "Chrome" / "Cef"
				info.ChildHWND = findContentChild(uintptr(hwnd), w, h)
				if info.ChildHWND != 0 {
					var childRect WinRect
					procGetWindowRect.Call(info.ChildHWND, uintptr(unsafe.Pointer(&childRect)))
					log.Printf("[窗口] 找到小程序内容区子窗口: HWND=%x 类名=%s 位置=(%d,%d) 尺寸=%dx%d",
						info.ChildHWND, getClassName(HWND(info.ChildHWND)),
						int(childRect.Left), int(childRect.Top),
						int(childRect.Right-childRect.Left), int(childRect.Bottom-childRect.Top))
				}

				found = info
				return 0
			}
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

// findContentChild 枚举子窗口，找到最可能是小程序内容区的那个
func findContentChild(parent uintptr, parentW, parentH int) uintptr {
	var bestChild uintptr
	var bestArea int

	callback := syscall.NewCallback(func(hwnd HWND, lParam uintptr) uintptr {
		ret, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if ret == 0 {
			return 1
		}

		var rect WinRect
		procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
		w := int(rect.Right - rect.Left)
		h := int(rect.Bottom - rect.Top)

		// 找面积最大且占父窗口大部分区域的子窗口
		area := w * h
		parentArea := parentW * parentH
		if area > bestArea && area > parentArea/4 && area < parentArea {
			bestArea = area
			bestChild = uintptr(hwnd)
		}
		return 1
	})
	procEnumChildWindows.Call(parent, callback, 0)
	return bestChild
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
					var clientRect WinRect
					procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&clientRect)))
					found = &WinInfo{
						HWND:    uintptr(hwnd),
						Title:   title,
						Class:   cls,
						ScreenX: int(rect.Left),
						ScreenY: int(rect.Top),
						ClientW: int(clientRect.Right),
						ClientH: int(clientRect.Bottom),
						W:       w, H: h,
						Visible: true,
					}
					found.ChildHWND = findContentChild(uintptr(hwnd), w, h)
					return 0
				}
			}
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

// ClickAt 通过句柄向窗口发送鼠标点击（纯 PostMessage，不抢鼠标）
// x, y 是相对于窗口客户区的坐标
func ClickAt(hwnd uintptr, childHWND uintptr, x, y int) {
	target := hwnd
	if childHWND != 0 {
		// 如果有子窗口，坐标需要换算到子窗口
		var parentRect, childRect WinRect
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&parentRect)))
		procGetWindowRect.Call(childHWND, uintptr(unsafe.Pointer(&childRect)))

		// 子窗口在屏幕上的偏移
		offsetX := int(childRect.Left - parentRect.Left)
		offsetY := int(childRect.Top - parentRect.Top)

		// 检查点击坐标是否在子窗口范围内
		cw := int(childRect.Right - childRect.Left)
		ch := int(childRect.Bottom - childRect.Top)
		rx := x - offsetX
		ry := y - offsetY
		if rx >= 0 && ry >= 0 && rx < cw && ry < ch {
			target = childHWND
			x, y = rx, ry
		}
	}

	lParam := uintptr((y << 16) | (x & 0xFFFF))

	// 先用 PostMessage（异步，不抢焦点）
	procPostMessageW.Call(target, WM_MOUSEMOVE, 0, lParam)
	procPostMessageW.Call(target, WM_LBUTTONDOWN, MK_LBUTTON, lParam)
	procPostMessageW.Call(target, WM_LBUTTONUP, 0, lParam)
}

// PressKey 向窗口发送按键
func PressKey(hwnd uintptr, keyCode int) {
	procPostMessageW.Call(hwnd, WM_KEYDOWN, uintptr(keyCode), 0)
	procPostMessageW.Call(hwnd, WM_KEYUP, uintptr(keyCode), 0)
}

// TypeText 向窗口发送文字输入
func TypeText(hwnd uintptr, text string) {
	for _, ch := range text {
		procPostMessageW.Call(hwnd, WM_CHAR, uintptr(ch), 0)
	}
}

// CaptureWindow 截图目标窗口
func CaptureWindow(hwnd uintptr) (image.Image, error) {
	var rect WinRect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return nil, fmt.Errorf("GetWindowRect 失败")
	}
	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("窗口尺寸无效: %dx%d", w, h)
	}

	// 优先从屏幕DC截取（对小程序最可靠）
	img, err := captureScreenRegion(int(rect.Left), int(rect.Top), w, h)
	if err == nil && !isBlankImage(img) {
		return img, nil
	}

	// 回退 PrintWindow
	dc, _, _ := procGetDC.Call(hwnd)
	if dc != 0 {
		defer procReleaseDC.Call(hwnd, dc)
		img, err := capturePW(hwnd, dc, w, h)
		if err == nil && !isBlankImage(img) {
			return img, nil
		}
		img, err = capturePWFull(hwnd, dc, w, h)
		if err == nil && !isBlankImage(img) {
			return img, nil
		}
	}

	return captureScreenRegion(int(rect.Left), int(rect.Top), w, h)
}

func captureScreenRegion(x, y, w, h int) (image.Image, error) {
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

	procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, uintptr(x), uintptr(y), SRCCOPY)
	return bitmapToImage(HBITMAP(bmp), w, h)
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

	ret, _, _ := procPrintWindow.Call(hwnd, memDC, 2)
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
	step := max(1, (w*h)/500)
	total := 0
	for y := 0; y < h; y += step {
		for x := 0; x < w; x += step {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 > 240 && g>>8 > 240 && b>>8 > 240 {
				whiteCount++
			}
			total++
		}
	}
	return total > 0 && float64(whiteCount)/float64(total) > 0.95
}

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
