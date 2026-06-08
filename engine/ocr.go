package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TessdataPath tesseract 语言包路径，启动时自动检测
var TessdataPath string

// TesseractPath tesseract 可执行文件完整路径
var TesseractPath string

func init() {
	// 自动检测 tesseract 可执行文件
	if p, err := exec.LookPath("tesseract"); err == nil {
		TesseractPath = p
	} else {
		// 常见安装路径
		for _, p := range []string{
			`C:\Program Files\Tesseract-OCR\tesseract.exe`,
			`C:\Program Files (x86)\Tesseract-OCR\tesseract.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				TesseractPath = p
				break
			}
		}
	}

	// 自动检测项目下的 tessdata 目录
	if d, err := os.Getwd(); err == nil {
		p := filepath.Join(d, "tessdata")
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			TessdataPath = p
		}
	}
}

// OCRResult OCR 识别结果
type OCRResult struct {
	Text    string
	Details []OCRWord
}

type OCRWord struct {
	Text                     string
	Confidence               float64
	Left, Top, Right, Bottom int
}

// OCRImage 对图片执行 OCR 识别（使用 stdout 模式，不依赖文件）
func OCRImage(img image.Image, lang string) (*OCRResult, error) {
	if img == nil {
		return nil, fmt.Errorf("ocr: 图片为空")
	}

	// 保存 PNG 到临时文件
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("hxocr_%d.png", time.Now().UnixNano()))
	defer os.Remove(tempFile)

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, fmt.Errorf("ocr: PNG编码失败: %v", err)
	}
	if err := os.WriteFile(tempFile, buf.Bytes(), 0644); err != nil {
		return nil, fmt.Errorf("ocr: 写入临时文件失败: %v", err)
	}

	// 构建 tesseract 命令: tesseract [options] input stdout
	tesseractBin := TesseractPath
	if tesseractBin == "" {
		tesseractBin = "tesseract"
	}

	args := []string{}
	if TessdataPath != "" {
		args = append(args, "--tessdata-dir", TessdataPath)
	}
	args = append(args, "-l", lang, "--psm", "6", tempFile, "stdout")

	cmd := exec.Command(tesseractBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ocr: tesseract 失败(tesseract=%s args=%v): %v (%s)",
			tesseractBin, args, err, string(out))
	}

	// stdout 直接就是识别文本
	text := strings.TrimSpace(string(out))
	return &OCRResult{Text: text}, nil
}

// OCRRegion 对完整图片的指定区域执行 OCR
func OCRRegion(fullImg image.Image, left, top, right, bottom int, lang string) (*OCRResult, error) {
	cropped := CropImage(fullImg, left, top, right, bottom)
	return OCRImage(cropped, lang)
}

// FindKeyword 在 OCR 结果中查找关键词
func FindKeyword(ocr *OCRResult, keywords []string) bool {
	cleaned := StripSpaces(strings.ToLower(ocr.Text))
	for _, kw := range keywords {
		if strings.Contains(cleaned, StripSpaces(strings.ToLower(kw))) {
			return true
		}
	}
	return false
}

// FindKeywordRegion 在指定区域中 OCR 并查找关键词
func FindKeywordRegion(img image.Image, region []int, keywords []string, lang string) (bool, string, error) {
	if len(region) != 4 {
		return false, "", fmt.Errorf("ocr: region 需要4个值 [left,top,right,bottom]")
	}
	result, err := OCRRegion(img, region[0], region[1], region[2], region[3], lang)
	if err != nil {
		return false, "", err
	}
	cleaned := StripSpaces(strings.ToLower(result.Text))
	for _, kw := range keywords {
		if strings.Contains(cleaned, StripSpaces(strings.ToLower(kw))) {
			return true, result.Text, nil
		}
	}
	return false, result.Text, nil
}
