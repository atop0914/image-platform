package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// ImageGenerator 图片生成器
type ImageGenerator struct {
	cfg        *ImageGenConfig
	generators map[string]*PlatformGenerator
}

// ImageGenConfig 图片生成配置
type ImageGenConfig struct {
	OutputDir  string
	LogDir     string
	Width      int
	Height     int
	MaxRetries int
	RetryDelay int
	Timeout    int
	MaxWorkers int
}

// PlatformConfig 平台配置 (从 config 导入)
type PlatformConfig struct {
	Name        string
	EnvKey      string
	APIKey      string
	URL         string
	Model       string
	Enabled     bool
	Description string
}

// PlatformGenerator 平台生成器
type PlatformGenerator struct {
	Name    string
	APIKey  string
	Model   string
	BaseURL string
}

// New 创建图片生成器
func New(cfg *ImageGenConfig, platforms map[string]PlatformConfig) *ImageGenerator {
	ig := &ImageGenerator{
		cfg:        cfg,
		generators: make(map[string]*PlatformGenerator),
	}

	for key, platformCfg := range platforms {
		gen := &PlatformGenerator{
			Name:    platformCfg.Name,
			APIKey:  platformCfg.APIKey,
			Model:   platformCfg.Model,
			BaseURL: platformCfg.URL,
		}
		ig.generators[key] = gen
		log.Printf("已启用平台: %s - %s", key, gen.Name)
	}

	return ig
}

// GenerateResult 生成结果
type GenerateResult struct {
	Platform    string
	FilePath    string
	ImageURL    string
	Success     bool
	Error       string
	GeneratedAt time.Time
}

// GenerateAll 并发生成所有平台的图片
func (g *ImageGenerator) GenerateAll(prompt string) []GenerateResult {
	if len(g.generators) == 0 {
		log.Println("没有已启用的平台")
		return nil
	}

	log.Println("========================================")
	log.Printf("🚀 开始生成任务: %s", prompt)
	log.Println("========================================")

	// 创建输出目录
	timestamp := time.Now().Format("20060102_150405")
	safePrompt := sanitizeFilename(prompt)
	outputDir := filepath.Join(g.cfg.OutputDir, fmt.Sprintf("%s_%s", timestamp, safePrompt))
	os.MkdirAll(outputDir, 0755)

	// 并发执行
	var wg sync.WaitGroup
	results := make([]GenerateResult, 0, len(g.generators))
	resultsChan := make(chan GenerateResult, len(g.generators))

	for key, gen := range g.generators {
		wg.Add(1)
		go func(platform string, generator *PlatformGenerator) {
			defer wg.Done()

			result := GenerateResult{
				Platform:    generator.Name,
				GeneratedAt: time.Now(),
			}

			startTime := time.Now()
			log.Printf("[%s] 开始生成...", generator.Name)

			imageURL, err := generator.Generate(prompt, g.cfg.Width, g.cfg.Height)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				log.Printf("[%s] 生成失败: %v", generator.Name, err)
			} else {
				filename := fmt.Sprintf("%s_%d.png", platform, time.Now().Unix())
				filepath := filepath.Join(outputDir, filename)

				if err := downloadImage(imageURL, filepath); err != nil {
					result.Success = false
					result.Error = err.Error()
					log.Printf("[%s] 下载失败: %v", generator.Name, err)
				} else {
					result.Success = true
					result.FilePath = filepath
					result.ImageURL = imageURL
					log.Printf("[%s] ✅ 生成成功: %s", generator.Name, filename)
				}
			}

			log.Printf("[%s] 耗时: %v", generator.Name, time.Since(startTime))
			resultsChan <- result
		}(key, gen)
	}

	wg.Wait()
	close(resultsChan)

	for result := range resultsChan {
		results = append(results, result)
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	log.Println("========================================")
	log.Printf("📊 生成完成: 成功 %d/%d", successCount, len(results))
	log.Println("========================================")

	return results
}

// GenerateSingle 生成单个平台图片
func (g *ImageGenerator) GenerateSingle(platform, prompt string) *GenerateResult {
	gen, ok := g.generators[platform]
	if !ok {
		return &GenerateResult{
			Platform:    platform,
			Success:    false,
			Error:      "平台未启用",
			GeneratedAt: time.Now(),
		}
	}

	startTime := time.Now()
	imageURL, err := gen.Generate(prompt, g.cfg.Width, g.cfg.Height)

	result := &GenerateResult{
		Platform:    gen.Name,
		GeneratedAt: time.Now(),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}

	// 下载图片
	timestamp := time.Now().Format("20060102_150405")
	safePrompt := sanitizeFilename(prompt)
	outputDir := filepath.Join(g.cfg.OutputDir, fmt.Sprintf("%s_%s", timestamp, safePrompt))
	os.MkdirAll(outputDir, 0755)

	filename := fmt.Sprintf("%s_%d.png", platform, time.Now().Unix())
	filepath := filepath.Join(outputDir, filename)

	if err := downloadImage(imageURL, filepath); err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}

	result.Success = true
	result.FilePath = filepath
	result.ImageURL = imageURL

	log.Printf("[%s] 生成成功，耗时: %v", gen.Name, time.Since(startTime))
	return result
}

// Generate 使用 HTTP 调用图片生成 API
func (p *PlatformGenerator) Generate(prompt string, width, height int) (string, error) {
	// 演示 langchaingo 调用
	p.callWithLangchaingo(prompt)

	// HTTP 调用
	return p.generateViaHTTP(prompt, width, height)
}

// callWithLangchaingo 使用 langchaingo 调用 LLM (演示)
func (p *PlatformGenerator) callWithLangchaingo(prompt string) {
	ctx := context.Background()
	llm, err := openai.New(
		openai.WithBaseURL(p.BaseURL),
		openai.WithModel(p.Model),
	)
	if err != nil {
		log.Printf("[%s] langchaingo 客户端创建: %v", p.Name, err)
		return
	}
	_, err = llms.GenerateFromSinglePrompt(ctx, llm, prompt)
	if err != nil {
		log.Printf("[%s] langchaingo 调用: %v", p.Name, err)
	}
}

// generateViaHTTP 直接 HTTP 调用
func (p *PlatformGenerator) generateViaHTTP(prompt string, width, height int) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	size := fmt.Sprintf("%dx%d", width, height)

	reqBody := map[string]interface{}{
		"model": p.Model,
		"prompt": prompt,
		"size":   size,
		"n":      1,
	}

	bodyBytes, _ := json.Marshal(reqBody)

	apiURL := p.BaseURL
	if !strings.Contains(apiURL, "/images/generations") {
		apiURL = apiURL + "/images/generations"
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Data) == 0 || result.Data[0].URL == "" {
		return "", fmt.Errorf("无图片返回: %s", string(respBody))
	}

	return result.Data[0].URL, nil
}

func sanitizeFilename(name string) string {
	if len(name) > 20 {
		name = name[:20]
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}

func downloadImage(url, filepath string) error {
	resp, err := httpGet(url)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, resp, 0644)
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Read(make([]byte, 0))
	return io.ReadAll(resp.Body)
}
