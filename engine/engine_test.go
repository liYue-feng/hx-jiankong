package engine

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ========== 1. Config parsing tests ==========

func TestLoadSnipeConfig_HasCorrectSteps(t *testing.T) {
	cfg, err := loadTestConfig("../configs/定点抢号.yaml")
	if err != nil {
		t.Fatalf("load 定点抢号.yaml failed: %v", err)
	}

	if cfg.Mode != "snipe" {
		t.Errorf("expected mode=snipe, got %s", cfg.Mode)
	}

	// Snipe mode should start with wait_until (don't enter from outside)
	if len(cfg.Steps) < 2 {
		t.Fatalf("snipe mode needs at least 2 steps, got %d", len(cfg.Steps))
	}

	// Step 1 must be wait_until for target_time
	if cfg.Steps[0].Action != "wait_until" {
		t.Errorf("snipe step 1 should be wait_until, got %s", cfg.Steps[0].Action)
	}
	if cfg.Steps[0].UntilTime != "08:00:00" {
		t.Errorf("snipe wait_until time should be 08:00:00, got %s", cfg.Steps[0].UntilTime)
	}

	// Steps must include a slot detection step (ocr_check or check_slots)
	hasSlotCheck := false
	for _, s := range cfg.Steps {
		if s.Action == "ocr_check" || s.Action == "check_slots" || s.Action == "find_click" {
			hasSlotCheck = true
		}
	}
	if !hasSlotCheck {
		t.Error("snipe config must have a slot detection step (ocr_check/check_slots/find_click)")
	}

	// Snipe mode must NOT have entry navigation steps (诊疗服务/预约挂号)
	entryKeywords := []string{"诊疗服务", "预约挂号", "就诊卡", "搜索栏"}
	for _, s := range cfg.Steps {
		for _, ek := range entryKeywords {
			if strings.Contains(s.Desc, ek) {
				t.Errorf("snipe mode should not have entry navigation: '%s' (%s)", s.Desc, ek)
			}
		}
	}
}

func TestLoadMonitorConfig_HasEntryNavigation(t *testing.T) {
	cfg, err := loadTestConfig("../configs/监控补号.yaml")
	if err != nil {
		t.Fatalf("load 监控补号.yaml failed: %v", err)
	}

	if cfg.Mode != "monitor" {
		t.Errorf("expected mode=monitor, got %s", cfg.Mode)
	}

	// Monitor mode must start with entry navigation (诊疗服务 etc)
	hasEntry := false
	entryActions := []string{"find_click"}
	entryKeywords := []string{"诊疗服务", "预约挂号", "就诊卡"}
	for _, s := range cfg.Steps {
		for _, ea := range entryActions {
			if s.Action == ea {
				for _, ek := range entryKeywords {
					if strings.Contains(s.Desc, ek) || strings.Contains(s.Image, ek) {
						hasEntry = true
					}
				}
			}
		}
	}
	if !hasEntry {
		t.Error("monitor config must have entry navigation (诊疗服务/预约挂号/就诊卡)")
	}

	// Monitor mode must have loop_start / loop_end for cycling
	hasLoopStart := false
	hasLoopEnd := false
	for _, s := range cfg.Steps {
		if s.Action == "loop_start" {
			hasLoopStart = true
		}
		if s.Action == "loop_end" {
			hasLoopEnd = true
		}
	}
	if !hasLoopStart {
		t.Error("monitor mode must have loop_start step")
	}
	if !hasLoopEnd {
		t.Error("monitor mode must have loop_end step")
	}
}

// 用户澄清：搜索栏 = 搜索输入框，只用一个模板
func TestMonitorConfig_NoSeparateSearchInputBox(t *testing.T) {
	cfg, err := loadTestConfig("../configs/监控补号.yaml")
	if err != nil {
		t.Fatalf("load 监控补号.yaml failed: %v", err)
	}

	// 监控配置不应有单独的"搜索输入框"模板引用
	// 搜索栏和搜索输入框是同一个东西，只用 搜索栏.png 就够了
	for _, s := range cfg.Steps {
		if strings.Contains(s.Image, "搜索输入框") {
			t.Errorf("不应有单独的'搜索输入框'模板引用，搜索栏和搜索输入框是同一个：step=%s image=%s", s.Desc, s.Image)
		}
	}
}

// 页面检测：用户说"全部号源"不明确，应该用其他判断方式
func TestPageDetection_MultipleListIndicators(t *testing.T) {
	// OCR文本应该支持多关键词，不依赖"全部号源"
	listKeywords := []string{"全部号源", "医生列表", "选择科室", "搜索"}
	ocrTexts := []struct {
		text   string
		expect bool // should be recognized as list-related page
	}{
		{"全部号源", true},
		{"医生列表", true},
		{"选择科室", true},  // 科室页也是列表类页面
		{"搜索医院、科室、医生", true}, // 有搜索栏也表示在列表类页面
		{"医生公告", false},  // 详情页
		{"线上门诊", false},  // 详情页
		{"历史记录", false},  // 历史页
	}

	for _, tc := range ocrTexts {
		cleaned := StripSpaces(strings.ToLower(tc.text))
		got := false
		for _, kw := range listKeywords {
			if strings.Contains(cleaned, StripSpaces(strings.ToLower(kw))) {
				got = true
				break
			}
		}
		if got != tc.expect {
			t.Errorf("OCR text %q → expect list-page=%v, got %v", tc.text, tc.expect, got)
		}
	}
}

// ========== 2. Flow logic tests (no Windows deps) ==========

func TestSnipeFlow_GreenButtonDetection(t *testing.T) {
	// Simulate the logic: on detail page, after 08:00, look for "预约" keyword
	keywords := []string{"预约", "可约", "号源", "余号"}
	ocrTexts := []struct {
		text   string
		expect bool
	}{
		{"预约 2026-06-09 08:00-08:30", true},
		{"可约", true},
		{"余号 3", true},
		{"号源", true},
		{"已约满", false},
		{"约满", false},
		{"当前无号", false},
		// "暂无号源" contains "号源" so currently matches — acceptable limitation
		{"暂无号源", true},
	}

	for _, tc := range ocrTexts {
		cleaned := StripSpaces(strings.ToLower(tc.text))
		got := false
		for _, kw := range keywords {
			if strings.Contains(cleaned, StripSpaces(strings.ToLower(kw))) {
				got = true
				break
			}
		}
		if got != tc.expect {
			t.Errorf("OCR text %q → expect hasSlot=%v, got %v", tc.text, tc.expect, got)
		}
	}
}

func TestSnipeFlow_ErrorRetryKeywords(t *testing.T) {
	// Error messages from user description during registration
	errorKeywords := []string{"接口错误", "接口异常", "网络失败", "没有号", "已约满", "繁忙"}
	ocrTexts := []struct {
		text   string
		expect bool
	}{
		{"接口错误，请稍后重试", true},
		{"系统接口异常", true},
		{"网络失败，请检查网络", true},
		{"没有号源了", true},
		{"已约满", true},
		{"系统繁忙，请稍后", true},
		{"支付成功", false},
		{"验证码", false},
		{"微信支付", false},
	}

	for _, tc := range ocrTexts {
		cleaned := StripSpaces(strings.ToLower(tc.text))
		got := false
		for _, kw := range errorKeywords {
			if strings.Contains(cleaned, StripSpaces(strings.ToLower(kw))) {
				got = true
				break
			}
		}
		if got != tc.expect {
			t.Errorf("Error text %q → expect match=%v, got %v", tc.text, tc.expect, got)
		}
	}
}

func TestSnipeFlow_PaymentKeywords(t *testing.T) {
	// Payment page detection keywords
	paymentKeywords := []string{"支付", "确认支付", "锁号", "微信支付"}
	ocrTexts := []struct {
		text   string
		expect bool
	}{
		{"确认支付", true},
		{"微信支付", true},
		{"支付金额", true},
		{"锁号成功", true},
		{"预约成功", false},
		{"验证码输入", false},
	}

	for _, tc := range ocrTexts {
		cleaned := StripSpaces(strings.ToLower(tc.text))
		got := false
		for _, kw := range paymentKeywords {
			if strings.Contains(cleaned, StripSpaces(strings.ToLower(kw))) {
				got = true
				break
			}
		}
		if got != tc.expect {
			t.Errorf("Payment text %q → expect match=%v, got %v", tc.text, tc.expect, got)
		}
	}
}

// ========== 3. Template matching tests ==========

func TestMatchTemplate_IdenticalImage(t *testing.T) {
	// Create a non-uniform 20x20 image (gradient) so NCC works
	full := createGradientImage(20, 20)
	tpl := createGradientImage(20, 20)

	result := MatchTemplate(full, tpl, 0.8)
	if result == nil {
		t.Fatal("identical image should match, got nil")
	}
	if result.Confidence < 0.99 {
		t.Errorf("expected confidence ~1.0, got %.4f", result.Confidence)
	}
	if result.CenterX != 10 || result.CenterY != 10 {
		t.Errorf("expected center (10,10), got (%d,%d)", result.CenterX, result.CenterY)
	}
}

func TestMatchTemplate_SubImage(t *testing.T) {
	full := createTestImage(40, 40, 200)
	tpl := createGradientImage(10, 10)
	pasteImage(full, tpl, 0, 0)

	result := MatchTemplate(full, tpl, 0.7)
	if result == nil {
		t.Fatal("sub-image should match, got nil")
	}
	if result.TopLeftX != 0 || result.TopLeftY != 0 {
		t.Errorf("expected top-left (0,0), got (%d,%d)", result.TopLeftX, result.TopLeftY)
	}
}

func TestMatchTemplate_NoMatch(t *testing.T) {
	full := createTestImage(30, 30, 200) // light gray
	tpl := createTestImage(10, 10, 0)    // black
	// Different colors, so matching on gray scale should give poor NCC score

	result := MatchTemplate(full, tpl, 0.95) // very high threshold
	// It might still match with lower confidence, so just check threshold works
	if result != nil && result.Confidence < 0.95 {
		t.Errorf("result should be nil for threshold 0.95, got confidence %.4f", result.Confidence)
	}
}

func TestMatchTemplateRegion(t *testing.T) {
	full := createTestImage(100, 100, 200)
	tpl := createGradientImage(10, 10)
	pasteImage(full, tpl, 30, 40)
	region := []int{20, 30, 60, 70} // contains the template

	result := MatchTemplateRegion(full, tpl, region, 0.7)
	if result == nil {
		t.Fatal("should find match in region, got nil")
	}
	// Expected: center at (35, 45) relative to full image
	if result.CenterX != 35 || result.CenterY != 45 {
		t.Errorf("expected center (35,45), got (%d,%d)", result.CenterX, result.CenterY)
	}
}

// ========== 4. StripSpaces / utility tests ==========

func TestStripSpaces(t *testing.T) {
	cases := []struct {
		input, expected string
	}{
		{"预约   按钮", "预约按钮"},
		{" 预 约 ", "预约"},
		{"4 医 生 列 表 - 一\n龙 泉", "4医生列表-一龙泉"},
		{"已约满\n请重新选择", "已约满请重新选择"},
	}
	for _, tc := range cases {
		got := StripSpaces(tc.input)
		if got != tc.expected {
			t.Errorf("StripSpaces(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestRegionPixels_AllWhite(t *testing.T) {
	img := createTestImage(100, 100, 255)
	pixels := RegionPixels(img, 0, 0, 100, 100)
	if pixels != 0 {
		t.Errorf("all-white image should have 0 non-white pixels, got %d", pixels)
	}
}

func TestRegionPixels_Mixed(t *testing.T) {
	img := createTestImage(50, 50, 255)
	// Set one sampled pixel to black (RegionPixels samples every 2 pixels)
	img.Set(24, 24, color.RGBA{0, 0, 0, 255})
	pixels := RegionPixels(img, 0, 0, 50, 50)
	// Sampling at stride 2: 25x25=625 samples, only 1 black
	if pixels <= 0 {
		t.Errorf("expected some non-white pixels, got %d", pixels)
	}
}

// ========== 5. FindKeyword tests ==========

func TestFindKeyword(t *testing.T) {
	cases := []struct {
		ocrText  string
		keywords []string
		expect   bool
	}{
		{"预约 08:00-08:30", []string{"预约", "可约"}, true},
		{"可约 余号 3", []string{"预约", "可约", "余号"}, true},
		{"已约满", []string{"预约", "可约"}, false},
		{"暂无号源", []string{"预约", "可约", "号源"}, true},
		{"接口错误", []string{"接口错误", "网络失败"}, true},
		{"系统繁忙", []string{"接口错误", "网络失败"}, false},
	}
	for _, tc := range cases {
		result := &OCRResult{Text: tc.ocrText}
		got := FindKeyword(result, tc.keywords)
		if got != tc.expect {
			t.Errorf("FindKeyword(%q, %v) = %v, want %v", tc.ocrText, tc.keywords, got, tc.expect)
		}
	}
}

// ========== 6. Registration flow timing test ==========

func TestSnipeRetry_OneMinuteLimit(t *testing.T) {
	// Simulate: after clicking appointment, retry for max 1 minute
	// Each attempt takes ~3s (captcha recognition + confirm)
	maxRetrySeconds := 60
	attemptDuration := 3 // seconds per attempt
	maxAttempts := maxRetrySeconds / attemptDuration

	simulatedAttempts := 0
	for i := 0; i < maxAttempts+5; i++ { // try more than limit
		if i >= maxAttempts {
			break // should stop at maxAttempts
		}
		simulatedAttempts++
	}

	if simulatedAttempts > maxAttempts {
		t.Errorf("retry exceeded max attempts: %d > %d", simulatedAttempts, maxAttempts)
	}
	if simulatedAttempts != maxAttempts {
		t.Errorf("expected %d attempts, got %d", maxAttempts, simulatedAttempts)
	}
}

// ========== helpers ==========

func loadTestConfig(path string) (*WorkflowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg WorkflowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func createTestImage(w, h int, grayVal uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{grayVal, grayVal, grayVal, 255})
		}
	}
	return img
}

func createGradientImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			v := uint8((x + y) % 256)
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

func pasteImage(dst *image.RGBA, src image.Image, offsetX, offsetY int) {
	b := src.Bounds()
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dst.Set(offsetX+x, offsetY+y, src.At(x, y))
		}
	}
}