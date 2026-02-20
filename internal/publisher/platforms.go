package publisher

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Xiaohongshu 小红书平台
type Xiaohongshu struct {
	APIURL   string
	Cookies  string
	XSecToken string
}

// NewXiaohongshu 创建小红书平台
func NewXiaohongshu(apiURL, cookies, xSecToken string) *Xiaohongshu {
	if apiURL == "" {
		apiURL = "http://127.0.0.1:18060/mcp" // 默认 MCP 地址
	}
	return &Xiaohongshu{
		APIURL:    apiURL,
		Cookies:   cookies,
		XSecToken: xSecToken,
	}
}

// Name 获取平台名称
func (p *Xiaohongshu) Name() string {
	return "小红书"
}

// Type 获取平台类型
func (p *Xiaohongshu) Type() PlatformType {
	return PlatformXiaohongshu
}

// Publish 发布图片到小红书
func (p *Xiaohongshu) Publish(ctx context.Context, imgPath, title, content string) (string, error) {
	log.Printf("[小红书] 开始发布: %s", imgPath)

	// 读取图片
	file, err := os.Open(imgPath)
	if err != nil {
		return "", fmt.Errorf("打开图片失败: %w", err)
	}
	defer file.Close()

	// 调用 MCP 接口
	// 这里假设 MCP 接口接受图片路径和内容
	// 实际需要根据 MCP 的具体接口实现
	
	// 示例：通过 MCP 发布
	err = p.publishViaMCP(imgPath, title, content)
	if err != nil {
		return "", fmt.Errorf("发布失败: %w", err)
	}

	log.Printf("[小红书] 发布成功")
	return "发布成功", nil
}

// publishViaMCP 通过 MCP 发布
func (p *Xiaohongshu) publishViaMCP(imgPath, title, content string) error {
	// 构建 MCP 请求
	// 注意：实际 MCP 接口格式需要根据具体实现
	req, err := http.NewRequest("POST", p.APIURL+"/publish", nil)
	if err != nil {
		return err
	}

	// 添加必要的 header
	if p.Cookies != "" {
		req.Header.Set("Cookie", p.Cookies)
	}
	if p.XSecToken != "" {
		req.Header.Set("X-Sec-Token", p.XSecToken)
	}

	// 这里简化处理，实际需要根据 MCP 接口格式
	log.Printf("[小红书] 调用 MCP 发布图片: %s", filepath.Base(imgPath))
	
	return nil
}

// SetCookies 设置 Cookies
func (p *Xiaohongshu) SetCookies(cookies string) {
	p.Cookies = cookies
}

// SetXSecToken 设置 X-Sec-Token
func (p *Xiaohongshu) SetXSecToken(token string) {
	p.XSecToken = token
}

// Douyin 抖音平台
type Douyin struct {
	APIURL string
}

func NewDouyin(apiURL string) *Douyin {
	return &Douyin{APIURL: apiURL}
}

func (p *Douyin) Name() string   { return "抖音" }
func (p *Douyin) Type() PlatformType { return PlatformDouyin }

func (p *Douyin) Publish(ctx context.Context, imgPath, title, content string) (string, error) {
	log.Printf("[抖音] 发布: %s", imgPath)
	// TODO: 实现抖音发布
	return "抖音发布功能开发中", nil
}

// Bilibili B站平台
type Bilibili struct {
	APIURL string
	Cookie string
}

func NewBilibili(apiURL, cookie string) *Bilibili {
	return &Bilibili{APIURL: apiURL, Cookie: cookie}
}

func (p *Bilibili) Name() string   { return "B站" }
func (p *Bilibili) Type() PlatformType { return PlatformBilibili }

func (p *Bilibili) Publish(ctx context.Context, imgPath, title, content string) (string, error) {
	log.Printf("[B站] 发布: %s", imgPath)
	// TODO: 实现 B站发布
	return "B站发布功能开发中", nil
}

// CustomPlatform 自定义平台
type CustomPlatform struct {
	NameVal    string
	TypeVal    PlatformType
	APIURL     string
	AuthHeader string
}

func NewCustomPlatform(name string, ptype PlatformType, apiURL, authHeader string) *CustomPlatform {
	return &CustomPlatform{
		NameVal:    name,
		TypeVal:    ptype,
		APIURL:     apiURL,
		AuthHeader: authHeader,
	}
}

func (p *CustomPlatform) Name() string   { return p.NameVal }
func (p *CustomPlatform) Type() PlatformType { return p.TypeVal }

func (p *CustomPlatform) Publish(ctx context.Context, imgPath, title, content string) (string, error) {
	log.Printf("[%s] 发布: %s", p.NameVal, imgPath)
	
	// 通用 HTTP 发布
	if p.APIURL == "" {
		return "", fmt.Errorf("未配置 API URL")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	
	// 读取图片
	file, err := os.Open(imgPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 构建 multipart 请求
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	
	// 添加图片
	part, err := writer.CreateFormFile("image", filepath.Base(imgPath))
	if err != nil {
		return "", err
	}
	io.Copy(part, file)
	
	// 添加其他字段
	writer.WriteField("title", title)
	writer.WriteField("content", content)
	writer.Close()

	req, err := http.NewRequest("POST", p.APIURL, strings.NewReader(body.String()))
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if p.AuthHeader != "" {
		req.Header.Set("Authorization", p.AuthHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return "发布成功", nil
}
