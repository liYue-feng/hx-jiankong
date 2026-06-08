package engine

import "fmt"

// stepCaptcha 验证码识别+输入流程
func (e *Engine) stepCaptcha(step WorkflowStep) {
	if e.Yunma == nil || !e.Yunma.IsConfigured() {
		e.Log("yunma captcha not configured, wait 30s for manual input")
		e.waitWithStopCheck(30)
		return
	}

	img, err := e.Capture()
	if err != nil {
		e.Log("captcha screenshot failed: %v", err)
		return
	}

	// default captcha region
	region := []int{100, 300, 300, 380}
	if step.Region != nil && len(step.Region) == 4 {
		region = step.Region
	}

	cropped := CropImage(img, region[0], region[1], region[2], region[3])
	e.Log("recognizing captcha...")

	result, err := e.Yunma.Recognize(cropped)
	if err != nil {
		e.Log("captcha recognition failed: %v", err)
		e.Notify("captcha failed", err.Error(), false)
		return
	}

	e.Log("captcha result: %s", result)

	// click input field to focus
	if step.X > 0 && step.Y > 0 {
		e.Click(step.X, step.Y)
		e.waitWithStopCheck(1)
	}

	// type captcha via PostMessage
	if e.WindowHWND != 0 {
		for _, ch := range result {
			PressKey(e.WindowHWND, int(ch))
		}
		e.waitWithStopCheck(1)
	}

	e.Log("captcha entered: %s", result)

	// click confirm button
	if step.Params != nil {
		if cxStr, ok := step.Params["confirm_x"]; ok {
			if cyStr, ok2 := step.Params["confirm_y"]; ok2 {
				var cxInt, cyInt int
				if _, err := fmt.Sscanf(cxStr, "%d", &cxInt); err == nil {
					if _, err := fmt.Sscanf(cyStr, "%d", &cyInt); err == nil {
						e.waitWithStopCheck(1)
						e.Click(cxInt, cyInt)
						e.Log("clicked confirm (%d,%d)", cxInt, cyInt)
					}
				}
			}
		}
	}
}

// stepCaptchaRetry 验证码重试（检测到错误后）
func (e *Engine) stepCaptchaRetry(step WorkflowStep) {
	e.Log("detected error, retrying captcha")

	// check if need to re-click appointment button
	if step.Keywords != nil {
		img, err := e.Capture()
		if err == nil {
			found, text, _ := FindKeywordRegion(img, step.Region, step.Keywords, e.Config.AppConfig.OCR.Language)
			if found {
				e.Log("found keyword %s, clicking appointment", text)
				if step.X > 0 && step.Y > 0 {
					e.Click(step.X, step.Y)
					e.waitWithStopCheck(3)
				}
			}
		}
	}

	// re-recognize captcha
	e.stepCaptcha(WorkflowStep{
		Region: step.Region,
		X:      step.X,
		Y:      step.Y,
		Params: step.Params,
	})
}
