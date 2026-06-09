package engine

import (
	"fmt"
	"image"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Engine 工作流引擎
type Engine struct {
	Config      *WorkflowConfig
	ConfigPath  string
	WindowHWND  uintptr
	State       WorkflowState
	CurrentStep int
	StepIdx     int
	LogChan     chan string
	StopChan    chan bool
	NotifyFunc  func(title, body string, urgent bool)

	// 运行时状态
	StartTime        time.Time
	lastSlotCheck    time.Time
	lastNotify       time.Time
	FoundSlot        bool
	consecutiveFails int

	// 定时器
	targetTimer *time.Timer

	// 步骤循环
	loopStart int
	inLoop    bool

		// 验证码识别
		Yunma *YunmaClient
}



// NewEngine 创建工作流引擎
func NewEngine(configPath string) (*Engine, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config WorkflowConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	if config.AppConfig.OCR.Language == "" {
		config.AppConfig.OCR.Language = "chi_sim"
	}
	if config.AppConfig.OCR.PSM == 0 {
		config.AppConfig.OCR.PSM = 6
	}
	if config.Schedule.RefreshMin == 0 {
		config.Schedule.RefreshMin = 30
	}
	if config.Schedule.RefreshMax == 0 {
		config.Schedule.RefreshMax = 180
	}
	if config.Schedule.NightStart == 0 {
		config.Schedule.NightStart = 1
	}
	if config.Schedule.NightEnd == 0 {
		config.Schedule.NightEnd = 6
	}
	if config.Schedule.SessionTimeout == 0 {
		config.Schedule.SessionTimeout = 15
	}
	if config.MaxOCRRetry == 0 {
		config.MaxOCRRetry = 60
	}

	return &Engine{
		Config:     &config,
		ConfigPath: configPath,
		State:      StateIdle,
		LogChan:    make(chan string, 100),
		StopChan:   make(chan bool, 1),
		Yunma:      NewYunmaClient(config.YunmaToken),
	}, nil
}

// Log 发送日志到通道
func (e *Engine) Log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	select {
	case e.LogChan <- msg:
	default:
	}
	log.Printf("[引擎] %s", msg)
}

// FindWindow 查找并缓存微信小程序窗口句柄
func (e *Engine) FindWindow() error {
	// 方式一：用类名过滤查找微信窗口
	win := FindWeChatWindow()
	if win != nil {
		e.WindowHWND = win.HWND
		e.Log("找到窗口: HWND=%x 标题=%s 类名=%s 尺寸=%dx%d", win.HWND, win.Title, win.Class, win.W, win.H)
		return nil
	}

	// 方式二：按标题关键词搜索（排除浏览器）
	title := e.Config.AppConfig.MiniAppTitle
	if title == "" {
		title = "微信"
	}
	for _, t := range []string{title, "微信", "小程序"} {
		win := FindWindowByTitle(t)
		if win != nil {
			e.WindowHWND = win.HWND
			e.Log("找到窗口(标题): HWND=%x 标题=%s 尺寸=%dx%d", win.HWND, win.Title, win.W, win.H)
			return nil
		}
	}
	return fmt.Errorf("未找到微信小程序窗口")
}

// Click 向窗口发送点击
func (e *Engine) Click(x, y int) {
	if e.WindowHWND == 0 {
		e.FindWindow()
	}
	if e.WindowHWND == 0 {
		e.Log("点击失败: 窗口句柄为空")
		return
	}
	ClickAt(e.WindowHWND, x, y)
	e.Log("点击 (%d,%d)", x, y)
}

// ClickTarget 点击目标
func (e *Engine) ClickTarget(t ClickTarget) {
	e.Click(t.X, t.Y)
}

// Capture 截图当前窗口
func (e *Engine) Capture() (image.Image, error) {
	if e.WindowHWND == 0 {
		if err := e.FindWindow(); err != nil {
			return nil, err
		}
	}
	return CaptureWindow(e.WindowHWND)
}

// DetectPage 检测当前页面类型
// 返回: "list"=医生列表页, "detail"=医生详情页, "history"=历史记录页, "unknown"=未知
func (e *Engine) DetectPage() string {
	img, err := e.Capture()
	if err != nil {
		e.Log("页面检测截图失败: %v", err)
		return "unknown"
	}

	titleRegion := e.Config.AppConfig.TitleRegion
	lang := e.Config.AppConfig.OCR.Language

	if len(titleRegion) == 4 {
		result, err := OCRRegion(img, titleRegion[0], titleRegion[1], titleRegion[2], titleRegion[3], lang)
		if err == nil {
			cleaned := StripSpaces(strings.ToLower(result.Text))
			e.Log("OCR顶部: %s", cleaned)

			// 详情页优先判断，避免“医生公告”里的“医生”误判为列表
			if strings.Contains(cleaned, "医生公告") || strings.Contains(cleaned, "线上门诊") {
				return "detail"
			}
			// 历史记录页
			if strings.Contains(cleaned, "历史记录") {
				return "history"
			}
			// 医生列表/科室搜索相关页面，不强依赖“全部号源”
			if strings.Contains(cleaned, "全部号源") || strings.Contains(cleaned, "医生列表") || strings.Contains(cleaned, "选择科室") || strings.Contains(cleaned, "搜索") {
				return "list"
			}
		}
	}

	return "unknown"
}

// Run 启动工作流
func (e *Engine) Run() {
	e.State = StateRunning
	e.CurrentStep = 0
	e.consecutiveFails = 0
	e.StartTime = time.Now()
	e.Log("开始执行工作流: %s (模式: %s)", e.Config.Name, e.Config.Mode)

	// 先找窗口
	if err := e.FindWindow(); err != nil {
		e.Log("警告: %v", err)
	}

	for e.State == StateRunning {
		select {
		case <-e.StopChan:
			e.State = StateStopped
			e.Log("工作流已停止")
			return
		default:
		}

		// 夜间休眠
		if e.isNightTime() {
			e.State = StateWaiting
			e.Log("夜间休眠 (%d:00-%d:00)，等待中...", e.Config.Schedule.NightStart, e.Config.Schedule.NightEnd)
			e.waitUntilMorning()
			e.State = StateRunning
			continue
		}

		// 执行当前步骤
		if e.CurrentStep >= len(e.Config.Steps) {
			// 所有步骤执行完毕
			if e.Config.Mode == "monitor" {
				// 监控模式：循环
				e.CurrentStep = e.loopStart
				e.Log("监控循环，回到步骤 %d", e.loopStart)
				e.randomWait(e.Config.Schedule.RefreshMin, e.Config.Schedule.RefreshMax)
				continue
			}
			break
		}

		step := e.Config.Steps[e.CurrentStep]
		e.executeStep(step)
		e.CurrentStep++
	}

	if e.State == StateRunning {
		e.State = StateIdle
	}
	e.Log("工作流结束")
}

func (e *Engine) executeStep(step WorkflowStep) {
	e.Log("[步骤%d] %s", e.CurrentStep+1, step.Desc)

	// 步骤前等待
	if step.WaitBefore > 0 {
		time.Sleep(time.Duration(step.WaitBefore) * time.Second)
	}

	action := strings.ToLower(step.Action)
	switch action {
	case "click":
		e.Click(step.X, step.Y)

	case "wait", "random_wait":
		secs := step.Timeout
		if secs <= 0 {
			secs = 2
		}
		if step.Interval != nil {
			secs = e.randomInt(step.Interval.Min, step.Interval.Max)
			e.Log("随机等待 %d 秒", secs)
		}
		e.waitWithStopCheck(secs)

	case "wait_until":
		e.Log("等待到 %s", step.UntilTime)
		e.waitUntilTime(step.UntilTime)

	case "ocr_check":
		e.stepOCRCheck(step)

	case "ocr_click":
		e.stepOCRClick(step)

	case "capture":
		img, err := e.Capture()
		if err != nil {
			e.Log("截图失败: %v", err)
		} else if step.Params != nil {
			if path, ok := step.Params["save"]; ok {
				SaveImage(img, path)
				e.Log("截图保存到 %s", path)
			}
		}

	case "goto":
		if step.Params != nil {
			if label, ok := step.Params["label"]; ok {
				e.Log("跳转到标签 %s", label)
				// 查找标签步骤
				for i, s := range e.Config.Steps {
					if s.Desc == label || (s.Params != nil && s.Params["label"] == label) {
						e.CurrentStep = i
						return
					}
				}
			}
			if loopTo, ok := step.Params["loop"]; ok {
				// loop=步骤描述，跳转到指定步骤
				for i, s := range e.Config.Steps {
					if s.Desc == loopTo {
						e.CurrentStep = i - 1 // -1 因为之后会 +1
						e.Log("循环到步骤 %d: %s", i, s.Desc)
						return
					}
				}
			}
		}

	case "success":
		e.runOnSuccess()

	case "notify":
		title := step.Desc
		body := ""
		if step.Params != nil {
			if t, ok := step.Params["title"]; ok {
				title = t
			}
			if b, ok := step.Params["body"]; ok {
				body = b
			}
		}
		urgent := step.Params != nil && step.Params["urgent"] == "true"
		e.Notify(title, body, urgent)

	case "exec":
		// 执行 shell 命令
		if step.Params != nil {
			if cmd, ok := step.Params["command"]; ok {
				e.Log("执行命令: %s", cmd)
				out, err := exec.Command("cmd", "/c", cmd).CombinedOutput()
				if err != nil {
					e.Log("命令执行失败: %v %s", err, string(out))
				}
			}
		}

	case "detect_page":
		page := e.DetectPage()
		e.Log("当前页面: %s", page)

	case "ensure_page":
		e.ensurePage(step)

	case "loop_start":
		e.loopStart = e.CurrentStep
		e.inLoop = true
		e.Log("标记循环起点: 步骤%d", e.CurrentStep)

	case "loop_end":
		if e.inLoop {
			e.CurrentStep = e.loopStart - 1 // -1 因为之后会 +1
			e.Log("循环回到步骤 %d", e.loopStart)
		}

	case "check_slots":
		e.checkSlots(step)

	case "recovery":
		e.runRecovery()

	case "subflow":
		e.Log("执行子流程 (%d 步骤)", len(step.SubSteps))
		for i, s := range step.SubSteps {
			e.Log("  [子步骤%d] %s", i+1, s.Desc)
			e.executeStep(s)
		}


	case "type_text":
		if step.Params != nil {
			if text, ok := step.Params["text"]; ok {
				// 替换模板变量
				text = strings.ReplaceAll(text, "{{doctor}}", e.Config.Doctor)
				text = strings.ReplaceAll(text, "{{patient}}", e.Config.Patient)
				text = strings.ReplaceAll(text, "{{department}}", e.Config.Department)
				e.Log("输入文字: %s", text)
				if e.WindowHWND == 0 {
					e.FindWindow()
				}
				TypeText(e.WindowHWND, text)
			}
		}

	case "find_click":
		e.stepFindClick(step)

	case "captcha":
		e.stepCaptcha(step)

	case "captcha_retry":
		e.stepCaptchaRetry(step)

	default:
		e.Log("未知动作: %s", action)
	}

	// 步骤后等待
	if step.WaitAfter > 0 {
		time.Sleep(time.Duration(step.WaitAfter) * time.Second)
	}
}

func (e *Engine) stepOCRCheck(step WorkflowStep) {
	img, err := e.Capture()
	if err != nil {
		e.Log("OCR截图失败: %v", err)
		return
	}

	found, text, err := FindKeywordRegion(img, step.Region, step.Keywords, e.Config.AppConfig.OCR.Language)
	if err != nil {
		e.Log("OCR识别失败: %v", err)
		return
	}

	cleaned := StripSpaces(strings.ToLower(text))
	e.Log("OCR结果: %s (关键词: %v) → 找到=%v", cleaned, step.Keywords, found)

	if found && step.OnFound != nil {
		e.consecutiveFails = 0
		e.Log("匹配到关键词，执行 on_found")
		if step.OnFound.NotifyUser {
			e.Notify(step.OnFound.Message, fmt.Sprintf("关键词: %v", step.Keywords), true)
		}
		for _, s := range step.OnFound.Steps {
			e.executeStep(s)
		}
		if step.OnFound.Loop {
			e.CurrentStep-- // 重复当前步骤
		}
	} else if !found && step.OnNotFound != nil {
		e.consecutiveFails++
		e.Log("未匹配到关键词(%d次)，执行 on_not_found", e.consecutiveFails)
		// 最大重试60次(约3-5分钟)后停止循环
		maxRetry := 60
		if e.Config.MaxOCRRetry > 0 {
			maxRetry = e.Config.MaxOCRRetry
		}
		if step.OnNotFound.Loop && e.consecutiveFails >= maxRetry {
			e.Log("OCR重试%d次后放弃，停止循环", maxRetry)
			e.Notify("OCR重试超时", fmt.Sprintf("连续%d次未找到关键词: %v", maxRetry, step.Keywords), false)
			step.OnNotFound.Loop = false
		}
		for _, s := range step.OnNotFound.Steps {
			e.executeStep(s)
		}
		if step.OnNotFound.Loop {
			e.CurrentStep-- // 重复当前步骤
		}
	}
}

func (e *Engine) stepOCRClick(step WorkflowStep) {
	img, err := e.Capture()
	if err != nil {
		e.Log("OCR点击截图失败: %v", err)
		return
	}

	found, _, err := FindKeywordRegion(img, step.Region, step.Keywords, e.Config.AppConfig.OCR.Language)
	if err != nil {
		e.Log("OCR识别失败: %v", err)
		return
	}

	if found {
		e.Log("找到 %v，点击指定位置 (%d,%d)", step.Keywords, step.X, step.Y)
		e.Click(step.X, step.Y)
	} else {
		e.Log("未找到 %v", step.Keywords)
		if step.OnNotFound != nil {
			for _, s := range step.OnNotFound.Steps {
				e.executeStep(s)
			}
		}
	}
}

// stepFindClick 模板匹配查找并点击
func (e *Engine) stepFindClick(step WorkflowStep) {
	if step.Image == "" {
		e.Log("find_click: image 为空")
		return
	}

	// 加载模板图片
	template, err := e.LoadTemplate(step.Image)
	if err != nil {
		e.Log("加载模板失败: %v", err)
		// 降级到坐标点击
		if step.X > 0 || step.Y > 0 {
			e.Log("降级到坐标点击 (%d,%d)", step.X, step.Y)
			e.Click(step.X, step.Y)
		}
		return
	}

	// 截图
	img, err := e.Capture()
	if err != nil {
		e.Log("find_click 截图失败: %v", err)
		return
	}

	// 匹配阈值
	threshold := step.MatchThreshold
	if threshold <= 0 {
		threshold = 0.8 // 默认0.8
	}

	var result *MatchResult
	if len(step.Region) == 4 {
		result = MatchTemplateRegion(img, template, step.Region, threshold)
	} else {
		result = MatchTemplate(img, template, threshold)
	}

	if result != nil {
		e.Log("模板匹配成功: %s (置信度=%.2f, 位置=%d,%d)", step.Image, result.Confidence, result.CenterX, result.CenterY)
		e.Click(result.CenterX, result.CenterY)

		if step.OnFound != nil {
			for _, s := range step.OnFound.Steps {
				e.executeStep(s)
			}
		}
	} else {
		e.Log("模板匹配失败: %s", step.Image)
		if step.OnNotFound != nil {
			for _, s := range step.OnNotFound.Steps {
				e.executeStep(s)
			}
			if step.OnNotFound.Loop {
				e.CurrentStep--
			}
			return
		}
		// 只有未配置 on_not_found 时才降级坐标点击，避免检测类步骤误点
		if step.X > 0 || step.Y > 0 {
			e.Log("降级到坐标点击 (%d,%d)", step.X, step.Y)
			e.Click(step.X, step.Y)
		}
	}
}

func (e *Engine) ensurePage(step WorkflowStep) {
	target := "list"
	if step.Params != nil {
		if t, ok := step.Params["target"]; ok {
			target = t
		}
	}

	for i := 0; i < 10; i++ {
		page := e.DetectPage()
		if page == target {
			e.Log("已到达目标页面: %s", target)
			return
		}

		switch page {
		case "detail":
			e.ClickTarget(e.Config.AppConfig.BackButton)
			e.Log("详情页 → 点返回")
		case "history":
			e.ClickTarget(e.Config.AppConfig.SearchButton)
			e.Log("历史记录页 → 点搜索")
		default:
			e.Log("未知页面 → 不操作，等待页面稳定")
		}
		time.Sleep(2 * time.Second)
	}
	e.Log("ensurePage: 重试10次后仍未到达 %s", target)
}

func (e *Engine) checkSlots(step WorkflowStep) {
	img, err := e.Capture()
	if err != nil {
		e.Log("检查号源截图失败: %v", err)
		return
	}

	e.lastSlotCheck = time.Now()
	sr := e.Config.AppConfig.ScheduleRegion
	found, text, err := FindKeywordRegion(img, sr, step.Keywords, e.Config.AppConfig.OCR.Language)
	if err != nil {
		e.Log("检查号源OCR失败: %v", err)
		return
	}

	e.Log("号源检查: %s → 有号=%v", StripSpaces(text), found)

	if found {
		e.FoundSlot = true
		e.Log("*** 发现号源! ***")
		e.Notify("发现号源!", fmt.Sprintf("识别到: %s", text), true)

		if step.OnFound != nil {
			for _, s := range step.OnFound.Steps {
				e.executeStep(s)
			}
		}

		// 执行成功流程
		e.runOnSuccess()
	} else {
		e.FoundSlot = false
		if step.OnNotFound != nil {
			for _, s := range step.OnNotFound.Steps {
				e.executeStep(s)
			}
		}
	}
}

func (e *Engine) runRecovery() {
	e.Log("执行超时恢复流程...")
	for _, step := range e.Config.Recovery {
		e.executeStep(step)
	}
}

func (e *Engine) runOnSuccess() {
	e.Log("执行挂号成功流程...")
	if e.Config.OnSuccess == nil {
		return
	}
	for _, step := range e.Config.OnSuccess {
		e.executeStep(step)
	}
	e.State = StateSuccess
}

func (e *Engine) Notify(title, body string, urgent bool) {
	e.Log("通知: %s - %s", title, body)
	if e.NotifyFunc != nil {
		e.NotifyFunc(title, body, urgent)
	}
	e.lastNotify = time.Now()
}

func (e *Engine) waitWithStopCheck(secs int) {
	for i := 0; i < secs; i++ {
		select {
		case <-e.StopChan:
			return
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func (e *Engine) waitUntilTime(timeStr string) {
	target, err := time.Parse("15:04:05", timeStr)
	if err != nil {
		target, err = time.Parse("15:04", timeStr)
		if err != nil {
			return
		}
	}
	now := time.Now()
	targetTime := time.Date(now.Year(), now.Month(), now.Day(),
		target.Hour(), target.Minute(), target.Second(), 0, now.Location())

	duration := time.Until(targetTime)
	if duration <= 0 {
		return
	}
	e.Log("等待 %v 到 %s", duration.Round(time.Second), timeStr)
	e.waitWithStopCheck(int(duration.Seconds()))
}

func (e *Engine) randomWait(minSec, maxSec int) {
	secs := e.randomInt(minSec, maxSec)
	e.Log("随机等待 %d 秒", secs)
	e.waitWithStopCheck(secs)
}

func (e *Engine) randomInt(min, max int) int {
	if min >= max {
		return min
	}
	return min + rand.Intn(max-min)
}

func (e *Engine) isNightTime() bool {
	hour := time.Now().Hour()
	ns := e.Config.Schedule.NightStart
	ne := e.Config.Schedule.NightEnd
	if ns < ne {
		return hour >= ns && hour < ne
	}
	return hour >= ns || hour < ne
}

func (e *Engine) waitUntilMorning() {
	for e.State == StateWaiting {
		if !e.isNightTime() {
			return
		}
		select {
		case <-e.StopChan:
			e.State = StateStopped
			return
		default:
			time.Sleep(30 * time.Second)
		}
	}
}

// Stop 停止工作流
func (e *Engine) Stop() {
	e.Log("收到停止信号")
	e.StopChan <- true
	e.State = StateStopped
}

// StartTime 返回启动时间（嵌入在engine中，从state类型取）
// 实际上需要在外层维护，这里提供便捷方法

// ReloadConfig 热更新配置
func (e *Engine) ReloadConfig() error {
	data, err := os.ReadFile(e.ConfigPath)
	if err != nil {
		return err
	}
	var config WorkflowConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}
	e.Config = &config
	e.Log("配置已热更新")
	return nil
}
