package engine

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// MatchResult 模板匹配结果
type MatchResult struct {
	CenterX    int     // 匹配区域中心X
	CenterY    int     // 匹配区域中心Y
	TopLeftX   int     // 左上角X
	TopLeftY   int     // 左上角Y
	Width      int     // 匹配区域宽
	Height     int     // 匹配区域高
	Confidence float64 // 置信度 0~1
}

// MatchTemplate NCC模板匹配，在全图中寻找模板图
// full: 全屏截图, template: 模板图, threshold: 阈值(0.7~0.95)
// 返回最佳匹配位置(nil=未找到)
func MatchTemplate(full, template image.Image, threshold float64) *MatchResult {
	fBounds := full.Bounds()
	tBounds := template.Bounds()
	fW, fH := fBounds.Dx(), fBounds.Dy()
	tW, tH := tBounds.Dx(), tBounds.Dy()

	if tW > fW || tH > fH {
		return nil
	}

	// 转为灰度数组（连续内存，访问更快）
	fGray := toGray(full)
	tGray := toGray(template)

	// 预计算模板均值和方差
	tLen := tW * tH
	tPixels := make([]float64, tLen)
	var tSum float64
	for i, v := range tGray {
		fv := float64(v)
		tPixels[i] = fv
		tSum += fv
	}
	tMean := tSum / float64(tLen)
	var tVarSum float64
	for _, v := range tPixels {
		d := v - tMean
		tVarSum += d * d
	}
	tStd := math.Sqrt(tVarSum)
	if tStd < 0.001 {
		return nil // 模板全是同一颜色，无法匹配
	}

	maxX, maxY := fW-tW, fH-tH

	// 自适应步进(大模板大步进加快速度,小模板精确扫描)
	stride := 2
	if tW > 80 || tH > 80 || maxX > 500 || maxY > 500 {
		stride = 4
	}
	if tW < 30 && tH < 30 {
		stride = 1
	}

	bestConf := -2.0
	bestX, bestY := 0, 0

	// 扫描所有位置
	for sy := 0; sy <= maxY; sy += stride {
		for sx := 0; sx <= maxX; sx += stride {
			// 计算当前区域均值
			var regionSum float64
			for ty := 0; ty < tH; ty++ {
				rowOff := (sy+ty)*fW + sx
				for tx := 0; tx < tW; tx++ {
					regionSum += float64(fGray[rowOff+tx])
				}
			}
			rMean := regionSum / float64(tLen)

			// NCC计算
			var num, denR, denT float64
			for ty := 0; ty < tH; ty++ {
				rowOff := (sy+ty)*fW + sx
				tRowOff := ty * tW
				for tx := 0; tx < tW; tx++ {
					fv := float64(fGray[rowOff+tx]) - rMean
					tv := tPixels[tRowOff+tx] - tMean
					num += fv * tv
					denR += fv * fv
					denT += tv * tv
				}
			}

			den := math.Sqrt(denR * denT)
			if den < 0.001 {
				continue
			}
			conf := num / den
			if conf > bestConf {
				bestConf = conf
				bestX, bestY = sx, sy
			}
		}
	}

	// 粗扫描后，如果置信度接近阈值，在最佳位置附近精扫
	if bestConf < threshold && bestConf > threshold-0.15 && stride > 1 {
		searchHalf := stride * 2
		xStart := max(0, bestX-searchHalf)
		yStart := max(0, bestY-searchHalf)
		xEnd := min(maxX, bestX+searchHalf)
		yEnd := min(maxY, bestY+searchHalf)

		for sy := yStart; sy <= yEnd; sy++ {
			for sx := xStart; sx <= xEnd; sx++ {
				var regionSum float64
				for ty := 0; ty < tH; ty++ {
					rowOff := (sy+ty)*fW + sx
					for tx := 0; tx < tW; tx++ {
						regionSum += float64(fGray[rowOff+tx])
					}
				}
				rMean := regionSum / float64(tLen)

				var num, denR, denT float64
				for ty := 0; ty < tH; ty++ {
					rowOff := (sy+ty)*fW + sx
					tRowOff := ty * tW
					for tx := 0; tx < tW; tx++ {
						fv := float64(fGray[rowOff+tx]) - rMean
						tv := tPixels[tRowOff+tx] - tMean
						num += fv * tv
						denR += fv * fv
						denT += tv * tv
					}
				}
				den := math.Sqrt(denR * denT)
				if den < 0.001 {
					continue
				}
				conf := num / den
				if conf > bestConf {
					bestConf = conf
					bestX, bestY = sx, sy
				}
			}
		}
	}

	if bestConf < threshold {
		return nil
	}

	return &MatchResult{
		CenterX:    bestX + tW/2,
		CenterY:    bestY + tH/2,
		TopLeftX:   bestX,
		TopLeftY:   bestY,
		Width:      tW,
		Height:     tH,
		Confidence: bestConf,
	}
}

// MatchTemplateRegion 在指定区域内进行模板匹配
func MatchTemplateRegion(full image.Image, template image.Image, region []int, threshold float64) *MatchResult {
	cropped := CropImage(full, region[0], region[1], region[2], region[3])
	result := MatchTemplate(cropped, template, threshold)
	if result == nil {
		return nil
	}
	return &MatchResult{
		CenterX:    result.CenterX + region[0],
		CenterY:    result.CenterY + region[1],
		TopLeftX:   result.TopLeftX + region[0],
		TopLeftY:   result.TopLeftY + region[1],
		Width:      result.Width,
		Height:     result.Height,
		Confidence: result.Confidence,
	}
}

// LoadTemplate 加载模板图片
func (e *Engine) LoadTemplate(path string) (image.Image, error) {
	if !filepath.IsAbs(path) {
		// 相对于项目根目录(baseDir)解析
		baseDir := filepath.Dir(e.ConfigPath)
		// configs/目录上一级是项目根
		if filepath.Base(baseDir) == "configs" {
			baseDir = filepath.Dir(baseDir)
		}
		path = filepath.Join(baseDir, path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开模板图片失败: %v (path=%s)", err, path)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("解码模板图片失败: %v", err)
	}
	return img, nil
}

// toGray 将 image.Image 转为灰度数组(逐行连续存储)
func toGray(img image.Image) []uint8 {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	gray := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		off := y * w
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			// 取高8位(0-255)再算亮度: L = 0.299R + 0.587G + 0.114B
			r8, g8, b8 := int(r>>8), int(g>>8), int(b>>8)
			lum := (299*r8 + 587*g8 + 114*b8) / 1000
			if lum > 255 {
				lum = 255
			}
			gray[off+x] = uint8(lum)
		}
	}
	return gray
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
