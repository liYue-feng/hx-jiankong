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
	Text       string
	Confidence float64
	Left, Top, Right, Bottom int
}

// OCRImage 对图片区域执行 OCR 识别
func OCRImage(img image.Image, lang string) (*OCRResult, error) {
	if img == nil {
		return nil, fmt.Errorf("ocr: 图片为空")
	}

	// 保存 PNG 到临时文件
	tempBase := filepath.Join(os.TempDir(), fmt.Sprintf("hxocr_%d", time.Now().UnixNano()))
	inputFile := tempBase + ".png"
	tsvFile := inputFile + ".tsv"
	defer os.Remove(inputFile)
	defer os.Remove(tsvFile)

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, fmt.Errorf("ocr: PNG编码失败: %v", err)
	}
	if err := os.WriteFile(inputFile, buf.Bytes(), 0644); err != nil {
		return nil, fmt.Errorf("ocr: 写入临时文件失败: %v", err)
	}

	// 构建 tesseract 命令
	tesseractBin := "tesseract"
	if TesseractPath != "" {
		tesseractBin = TesseractPath
	}

	args := []string{tesseractBin}
	if TessdataPath != "" {
		args = append(args, "--tessdata-dir", TessdataPath)
	}
	args = append(args, inputFile, tempBase, "-l", lang, "--psm", "6", "tsv")

	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ocr: tesseract 失败: %v (%s)", err, string(out))
	}

	// 读取 TSV 结果
	return parseTSV(tsvFile)
}

// OCRRegion 对完整图片的指定区域执行 OCR
func OCRRegion(fullImg image.Image, left, top, right, bottom int, lang string) (*OCRResult, error) {
	cropped := CropImage(fullImg, left, top, right, bottom)
	return OCRImage(cropped, lang)
}

func parseTSV(tsvPath string) (*OCRResult, error) {
	data, err := os.ReadFile(tsvPath)
	if err != nil {
		return nil, fmt.Errorf("ocr: 读取TSV失败: %v", err)
	}

	result := &OCRResult{}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}
		text := fields[11]
		if text == "" {
			continue
		}
		result.Text += text + " "
	}

	result.Text = strings.TrimSpace(result.Text)
	return result, nil
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
