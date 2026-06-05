package notify

import (
	"fmt"

	serverchan "github.com/easychen/serverchan-sdk-golang"
)

// Notifier 通知器
type Notifier struct {
	SCTKey string
}

// NewNotifier 创建通知器
func NewNotifier(sctKey string) *Notifier {
	return &Notifier{SCTKey: sctKey}
}

// Send 发送通知
func (n *Notifier) Send(title, body string, urgent bool) error {
	if n.SCTKey == "" {
		return fmt.Errorf("SCT Key 未配置")
	}

	tags := ""
	if urgent {
		tags = "华医通|有号"
	}

	resp, err := serverchan.ScSend(n.SCTKey, title, body, &serverchan.ScSendOptions{
		Tags: tags,
	})
	if err != nil {
		return fmt.Errorf("Server酱推送失败: %v", err)
	}
	if resp != nil && resp.Code != 0 {
		return fmt.Errorf("Server酱返回错误: %s", resp.Message)
	}
	return nil
}

// SendWithImage 发送带图片的通知
func (n *Notifier) SendWithImage(title, body, imagePath string, urgent bool) error {
	if n.SCTKey == "" {
		return fmt.Errorf("SCT Key 未配置")
	}

	tags := ""
	if urgent {
		tags = "华医通|有号|截图"
	}

	// 使用 Markdown 格式发送图片链接
	markdown := fmt.Sprintf("# %s\n\n%s\n\n![截图](%s)", title, body, imagePath)

	resp, err := serverchan.ScSend(n.SCTKey, title, markdown, &serverchan.ScSendOptions{
		Tags: tags,
	})
	if err != nil {
		return fmt.Errorf("Server酱推送失败: %v", err)
	}
	if resp != nil && resp.Code != 0 {
		return fmt.Errorf("Server酱返回错误: %s", resp.Message)
	}
	return nil
}

// IsConfigured 检查通知是否已配置
func (n *Notifier) IsConfigured() bool {
	return n.SCTKey != ""
}
