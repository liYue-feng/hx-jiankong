package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os/exec"
	"strings"
	"time"
)

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

	// 编码为 PNG
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, fmt.Errorf("ocr: PNG编码失败: %v", err)
	}

	// 调用 tesseract
	tempFile := fmt.Sprintf("ocr_temp_%d", time.Now().UnixNano())
	defer exec.Command("cmd", "/c", "del", tempFile+".png", tempFile, tempFile+".tsv").Run()

	// 保存临时文件
	cmd := exec.Command("cmd", "/c", "tesseract", "stdin", tempFile, "-l", lang, "--psm", "6", "tsv")
	cmd.Stdin = bytes.NewReader(buf.Bytes())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ocr: tesseract 失败: %v (%s)", err, string(out))
	}

	// 读取 TSV 结果
	return parseTSV(tempFile + ".tsv")
}

// OCRRegion 对完整图片的指定区域执行 OCR
func OCRRegion(fullImg image.Image, left, top, right, bottom int, lang string) (*OCRResult, error) {
	cropped := CropImage(fullImg, left, top, right, bottom)
	return OCRImage(cropped, lang)
}

func parseTSV(tsvPath string) (*OCRResult, error) {
	cmd := exec.Command("cmd", "/c", "type", tsvPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ocr: 读取TSV失败: %v", err)
	}

	result := &OCRResult{}
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 || line == "" {
			continue // 跳过表头
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
