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
	CenterX    int
	CenterY    int
	TopLeftX   int
	TopLeftY   int
	Width      int
	Height     int
	Confidence float64
}

// MatchTemplate NCC模板匹配，在全图中寻找模板图
// 支持多尺度匹配（0.7x~2.0x），适配不同DPI缩放
func MatchTemplate(full, template image.Image, threshold float64) *MatchResult {
	// 先尝试原始尺寸
	result := matchTemplateNCC(full, template, threshold)
	if result != nil {
		return result
	}

	// 多尺度匹配
	scales := []float64{0.7, 0.8, 0.9, 1.1, 1.2, 1.5, 2.0}
	tBounds := template.Bounds()
	tW, tH := tBounds.Dx(), tBounds.Dy()

	for _, scale := range scales {
		sw := int(float64(tW) * scale)
		sh := int(float64(tH) * scale)
		if sw < 10 || sh < 10 || sw > 500 || sh > 500 {
			continue
		}
		scaled := resizeImage(template, sw, sh)
		result := matchTemplateNCC(full, scaled, threshold)
		if result != nil {
			// 把匹配位置映射回原模板尺寸
			result.Width = tW
			result.Height = tH
			result.CenterX = result.CenterX - (sw-tW)/2
			result.CenterY = result.CenterY - (sh-tH)/2
			return result
		}
	}

	return nil
}

// matchTemplateNCC 单尺度 NCC 匹配
func matchTemplateNCC(full, template image.Image, threshold float64) *MatchResult {
	fBounds := full.Bounds()
	tBounds := template.Bounds()
	fW, fH := fBounds.Dx(), fBounds.Dy()
	tW, tH := tBounds.Dx(), tBounds.Dy()

	if tW > fW || tH > fH {
		return nil
	}

	fGray := toGray(full)
	tGray := toGray(template)

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
		return nil
	}

	maxX, maxY := fW-tW, fH-tH

	stride := 2
	if tW > 80 || tH > 80 || maxX > 500 || maxY > 500 {
		stride = 4
	}
	if tW < 30 && tH < 30 {
		stride = 1
	}

	bestConf := -2.0
	bestX, bestY := 0, 0

	for sy := 0; sy <= maxY; sy += stride {
		for sx := 0; sx <= maxX; sx += stride {
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

	// 精扫
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

// resizeImage 简单近邻缩放
func resizeImage(img image.Image, newW, newH int) image.Image {
	bounds := img.Bounds()
	oldW, oldH := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		srcY := y * oldH / newH
		for x := 0; x < newW; x++ {
			srcX := x * oldW / newW
			dst.Set(x, y, img.At(srcX+bounds.Min.X, srcY+bounds.Min.Y))
		}
	}
	return dst
}

// MatchTemplateRegion 在指定区域内进行模板匹配
func MatchTemplateRegion(full, template image.Image, region []int, threshold float64) *MatchResult {
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
		baseDir := filepath.Dir(e.ConfigPath)
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

func toGray(img image.Image) []uint8 {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	gray := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		off := y * w
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
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
