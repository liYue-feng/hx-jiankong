package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// YunmaClient 云码验证码识别客户端
type YunmaClient struct {
	Token   string
	BaseURL string
	Client  *http.Client
}

// YunmaResponse 云码API响应
type YunmaResponse struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg,omitempty"`
	ID   string         `json:"id,omitempty"`
	Data *YunmaData     `json:"data,omitempty"`
}

type YunmaData struct {
	Result string `json:"result"`
}

// NewYunmaClient 创建云码客户端
func NewYunmaClient(token string) *YunmaClient {
	return &YunmaClient{
		Token:   token,
		BaseURL: "http://api.ymocr.com:8081",
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Recognize 识别验证码图片，返回识别文本
func (y *YunmaClient) Recognize(img image.Image) (string, error) {
	// 将图片编码为PNG字节流
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("验证码图片编码失败: %v", err)
	}

	// 构建 multipart/form-data 请求
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// 添加图片文件字段
	part, err := writer.CreateFormFile("image", "captcha.png")
	if err != nil {
		return "", fmt.Errorf("创建表单字段失败: %v", err)
	}
	if _, err := io.Copy(part, &buf); err != nil {
		return "", fmt.Errorf("写入图片数据失败: %v", err)
	}

	// 添加 token 字段
	writer.WriteField("token", y.Token)
	// 添加类型: 1004 = 4-5位数字字母混合
	writer.WriteField("type", "1004")

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("关闭表单失败: %v", err)
	}

	// 发送请求
	req, err := http.NewRequest("POST", y.BaseURL+"/ocr/begin", body)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := y.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求云码API失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result YunmaResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析云码响应失败: %v (body: %s)", err, string(respBody))
	}

	if result.Code != 0 {
		return "", fmt.Errorf("云码返回错误: code=%d msg=%s", result.Code, result.Msg)
	}

	// 有ID说明需要轮询结果
	if result.ID != "" {
		return y.pollResult(result.ID)
	}

	// 直接返回结果
	if result.Data != nil && result.Data.Result != "" {
		return strings.TrimSpace(result.Data.Result), nil
	}

	return "", fmt.Errorf("云码未返回识别结果")
}

// pollResult 轮询识别结果
func (y *YunmaClient) pollResult(id string) (string, error) {
	for i := 0; i < 30; i++ {
		body := fmt.Sprintf("id=%s&token=%s", id, y.Token)
		req, err := http.NewRequest("POST", y.BaseURL+"/ocr/result",
			strings.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := y.Client.Do(req)
		if err != nil {
			return "", err
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result YunmaResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", err
		}

		if result.Code == 0 && result.Data != nil && result.Data.Result != "" {
			return strings.TrimSpace(result.Data.Result), nil
		}

		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("轮询验证码结果超时")
}

// IsConfigured 检查是否已配置
func (y *YunmaClient) IsConfigured() bool {
	return y != nil && y.Token != "" && y.Token != "YOUR_YUNMA_TOKEN_HERE"
}
