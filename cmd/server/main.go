package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"image-platform/internal/publisher"
)

// ========== 配置 ==========
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	ImageGen   ImageGenConfig  `yaml:"imageGen"`
	Platforms  PlatformConfigs `yaml:"platforms"`
	Publish    PublishConfig   `yaml:"publish"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
}

type ImageGenConfig struct {
	OutputDir  string `yaml:"outputDir"`
	LogDir     string `yaml:"logDir"`
	Width      int    `yaml:"width"`
	Height     int    `yaml:"height"`
}

type PlatformConfigs map[string]PlatformConfig

type PlatformConfig struct {
	Name        string `yaml:"name"`
	EnvKey      string `yaml:"envKey"`
	APIKey      string `yaml:"apiKey"`
	URL         string `yaml:"url"`
	Model       string `yaml:"model"`
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description"`
}

type PublishConfig struct {
	Xiaohongshu struct {
		Enabled    bool   `yaml:"enabled"`
		MCPURL     string `yaml:"mcpUrl"`
		Cookies    string `yaml:"cookies"`
		XSecToken  string `yaml:"xSecToken"`
	} `yaml:"xiaohongshu"`
	Douyin struct {
		Enabled bool   `yaml:"enabled"`
	} `yaml:"douyin"`
	Bilibili struct {
		Enabled bool   `yaml:"enabled"`
		Cookie  string `yaml:"cookie"`
	} `yaml:"bilibili"`
}

// ========== 数据模型 ==========
type ImageRecord struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"size:255;not null" json:"name"`
	Date         string     `gorm:"size:20;not null" json:"date"`
	Path         string     `gorm:"size:512;not null" json:"path"`
	Platform     string     `gorm:"size:50;not null" json:"platform"`
	Model        string     `gorm:"size:100;not null" json:"model"`
	Prompt       string     `gorm:"size:1000" json:"prompt"`
	GeneratedAt  time.Time  `gorm:"not null" json:"generated_at"`
	Status       string     `gorm:"size:20;default:'pending'" json:"status"`
	Note         string     `gorm:"type:text" json:"note"`
	ModeratedAt  *time.Time `json:"moderated_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (ImageRecord) TableName() string {
	return "images"
}

// ========== 全局变量 ==========
var db *gorm.DB
var cfg *Config
var pubManager *publisher.Manager

func main() {
	configPath := flag.String("c", "config/config.yaml", "配置文件")
	flag.Parse()
	godotenv.Load("config/.env")

	var err error
	cfg, err = loadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.DBName)

	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	db.AutoMigrate(&ImageRecord{})
	os.MkdirAll(cfg.ImageGen.OutputDir, 0755)
	setupLogging()

	// 初始化发布管理器
	pubManager = initPublisher()

	for key, p := range cfg.Platforms {
		if p.Enabled && p.APIKey != "" {
			log.Printf("已启用平台: %s - %s", key, p.Name)
		}
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.LoadHTMLGlob("web/templates/*")
	r.Static("/static", "./web")
	r.Static("/images", cfg.ImageGen.OutputDir) // 图片目录

	// 页面路由
	r.GET("/", index)
	r.GET("/add", addPage)
	r.GET("/moderate/:id", moderatePage)
	r.GET("/records", recordsPage)
	r.GET("/gallery", galleryPage) // 当天图库

	// API 路由
	r.POST("/api/generate", handleGenerate)
	r.GET("/api/images", listImages)
	r.POST("/api/moderate", moderateImage)
	r.GET("/api/records", listRecords)
	r.DELETE("/api/images/:id", deleteImage)
	r.GET("/api/report", dailyReport)
	r.GET("/api/gallery", getGallery) // 当天图库 API
	r.POST("/api/publish", handlePublish) // 发布 API
	r.GET("/api/platforms", listPlatforms) // 平台列表

	log.Printf("🚀 图片平台启动于端口 %s", cfg.Server.Port)
	r.Run(":" + cfg.Server.Port)
}

// ========== 页面处理 ==========
func index(c *gin.Context) {
	var pending, approved, rejected []ImageRecord
	db.Where("status = ?", "pending").Limit(100).Find(&pending)
	db.Where("status = ?", "approved").Limit(100).Find(&approved)
	db.Where("status = ?", "rejected").Limit(100).Find(&rejected)

	c.HTML(http.StatusOK, "index.html", gin.H{
		"records":      pending,
		"total":        len(pending),
		"approved":     len(approved),
		"rejected":     len(rejected),
		"pendingCount": len(pending),
	})
}

func addPage(c *gin.Context) {
	c.HTML(http.StatusOK, "add.html", nil)
}

func moderatePage(c *gin.Context) {
	var record ImageRecord
	if err := db.First(&record, c.Param("id")).Error; err != nil {
		c.String(http.StatusNotFound, "Image not found")
		return
	}
	c.HTML(http.StatusOK, "moderate.html", gin.H{"record": record})
}

func recordsPage(c *gin.Context) {
	var records []ImageRecord
	db.Order("generated_at DESC").Limit(100).Find(&records)
	c.HTML(http.StatusOK, "records.html", gin.H{"records": records, "total": len(records)})
}

// ========== 当天图库页面 ==========
func galleryPage(c *gin.Context) {
	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	var records []ImageRecord
	db.Where("date = ? AND status = ?", date, "approved").Order("generated_at DESC").Find(&records)
	c.HTML(http.StatusOK, "gallery.html", gin.H{
		"records": records,
		"date":    date,
		"total":   len(records),
	})
}

// ========== API 处理 ==========
func handleGenerate(c *gin.Context) {
	var req struct {
		Prompt   string `json:"prompt" binding:"required"`
		Platform string `json:"platform" binding:"required"` // 必选
		Size     string `json:"size"`                        // 可选，如 "1920x1080"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "请指定平台: " + err.Error()})
		return
	}

	// 生成图片
	result := generateImage(req.Platform, req.Prompt, req.Size)

	if result == nil {
		c.JSON(500, gin.H{"error": "生成失败，请检查平台是否正确或API是否配置"})
		return
	}

	genTime := time.Now()
	record := ImageRecord{
		Name:        result.Filename,
		Date:        genTime.Format("2006-01-02"),
		Path:        result.FilePath,
		Platform:    result.Platform,
		Model:       result.Model,
		Prompt:      req.Prompt,
		GeneratedAt: genTime,
		Status:      "pending",
	}
	db.Create(&record)

	c.JSON(200, gin.H{"message": "success", "filePath": result.FilePath, "platform": result.Platform, "model": result.Model})
}

func listImages(c *gin.Context) {
	var records []ImageRecord
	query := db.Model(&ImageRecord{})
	if s := c.DefaultQuery("status", "all"); s != "all" {
		query = query.Where("status = ?", s)
	}
	query.Order("generated_at DESC").Limit(100).Find(&records)
	c.JSON(200, gin.H{"records": records, "total": len(records)})
}

func moderateImage(c *gin.Context) {
	var req struct {
		ID     uint   `json:"id" binding:"required"`
		Status string `json:"status" binding:"required"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	db.Model(&ImageRecord{}).Where("id = ?", req.ID).Updates(map[string]interface{}{
		"status": req.Status, "note": req.Note, "moderated_at": time.Now()})
	c.JSON(200, gin.H{"message": "success"})
}

func listRecords(c *gin.Context) {
	var records []ImageRecord
	db.Order("generated_at DESC").Limit(100).Find(&records)
	c.JSON(200, gin.H{"records": records, "total": len(records)})
}

func deleteImage(c *gin.Context) {
	db.Delete(&ImageRecord{}, c.Param("id"))
	c.JSON(200, gin.H{"message": "success"})
}

func dailyReport(c *gin.Context) {
	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	var records []ImageRecord
	db.Where("date = ?", date).Find(&records)

	approved, rejected, pending := 0, 0, 0
	platformStats := make(map[string]int)
	for _, r := range records {
		switch r.Status {
		case "approved": approved++
		case "rejected": rejected++
		default: pending++
		}
		platformStats[r.Platform]++
	}
	c.JSON(200, gin.H{
		"date":     date,
		"total":    len(records),
		"approved": approved,
		"rejected": rejected,
		"pending":  pending,
		"platform_stats": platformStats,
		"images":   records,
	})
}

// ========== 图库 API ==========
func getGallery(c *gin.Context) {
	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	var records []ImageRecord
	db.Where("date = ? AND status = ?", date, "approved").Order("generated_at DESC").Find(&records)
	c.JSON(200, gin.H{"records": records, "total": len(records), "date": date})
}

// ========== 发布 API ==========
func handlePublish(c *gin.Context) {
	var req struct {
		ImageID   uint     `json:"image_id" binding:"required"`
		Platforms []string `json:"platforms"` // 发布到哪些平台，空表示所有
		Title     string   `json:"title"`
		Content   string   `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// 获取图片信息
	var record ImageRecord
	if err := db.First(&record, req.ImageID).Error; err != nil {
		c.JSON(404, gin.H{"error": "图片不存在"})
		return
	}

	if record.Status != "approved" {
		c.JSON(400, gin.H{"error": "只能发布审核通过的图片"})
		return
	}

	ctx := context.Background()
	results := make(map[string]string)

	// 确定要发布的平台
	platformsToUse := req.Platforms
	if len(platformsToUse) == 0 {
		for _, p := range pubManager.List() {
			platformsToUse = append(platformsToUse, string(p.Type()))
		}
	}

	// 发布到各平台
	for _, plat := range platformsToUse {
		url, err := pubManager.Publish(publisher.PlatformType(plat), ctx, record.Path, req.Title, req.Content)
		if err != nil {
			results[plat] = "失败: " + err.Error()
		} else {
			results[plat] = url
		}
	}

	c.JSON(200, gin.H{"message": "success", "results": results})
}

// ========== 平台列表 API ==========
func listPlatforms(c *gin.Context) {
	platforms := getEnabledPlatforms()
	result := make([]map[string]interface{}, 0, len(platforms))
	for key, p := range platforms {
		result = append(result, map[string]interface{}{
			"id":          key,
			"name":        p.Name,
			"model":       p.Model,
			"description": p.Description,
			"enabled":     p.Enabled,
		})
	}
	c.JSON(200, gin.H{"platforms": result})
}

// ========== 工具函数 ==========
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.ImageGen.Width == 0 {
		c.ImageGen.Width = 1024
	}
	if c.ImageGen.Height == 0 {
		c.ImageGen.Height = 2048
	}
	for key, p := range c.Platforms {
		if apiKey := os.Getenv(p.EnvKey); apiKey != "" {
			p.APIKey, p.Enabled = apiKey, true
		}
		c.Platforms[key] = p
	}
	return &c, nil
}

func getEnabledPlatforms() map[string]PlatformConfig {
	result := make(map[string]PlatformConfig)
	for key, p := range cfg.Platforms {
		if p.Enabled && p.APIKey != "" {
			result[key] = p
		}
	}
	return result
}

func setupLogging() {
	os.MkdirAll(cfg.ImageGen.LogDir, 0755)
	logFile := fmt.Sprintf("%s/app_%s.log", cfg.ImageGen.LogDir, time.Now().Format("20060102"))
	f, _ := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	log.SetOutput(f)
}

// ========== 初始化发布管理器 ==========
func initPublisher() *publisher.Manager {
	mgr := publisher.New()

	// 注册小红书
	if cfg.Publish.Xiaohongshu.Enabled {
		mgr.Register(publisher.NewXiaohongshu(
			cfg.Publish.Xiaohongshu.MCPURL,
			cfg.Publish.Xiaohongshu.Cookies,
			cfg.Publish.Xiaohongshu.XSecToken,
		))
	}

	// 注册抖音
	if cfg.Publish.Douyin.Enabled {
		mgr.Register(publisher.NewDouyin(""))
	}

	// 注册 B站
	if cfg.Publish.Bilibili.Enabled {
		mgr.Register(publisher.NewBilibili("", cfg.Publish.Bilibili.Cookie))
	}

	return mgr
}

// ========== 图片生成 ==========
type GenerateResult struct {
	Platform string
	Model    string
	Filename string
	FilePath string
	Success  bool
}

func generateImage(platform, prompt, size string) *GenerateResult {
	p, ok := cfg.Platforms[platform]
	if !ok || !p.Enabled {
		return nil
	}

	// 阿里云百炼是异步 API
	if platform == "aliyun" {
		return generateAliyunImage(p, prompt)
	}

	// 魔塔社区是异步 API，支持 size 参数
	if platform == "modelscope" {
		return generateModelScopeImage(p, prompt, size)
	}

	// 其他平台使用同步 API (SiliconFlow, OpenAI)
	return generateSyncImage(p, prompt)
}

// 同步图片生成 (SiliconFlow, OpenAI)
func generateSyncImage(p PlatformConfig, prompt string) *GenerateResult {
	client := &http.Client{Timeout: 120 * time.Second}
	width, height := cfg.ImageGen.Width, cfg.ImageGen.Height
	
	// 如果高度是宽度的2倍（竖图），需要调整
	size := fmt.Sprintf("%dx%d", width, height)
	if height > width {
		size = fmt.Sprintf("%dx%d", width/2, height)
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": p.Model, "prompt": prompt, "size": size, "n": 1,
	})

	apiURL := p.URL
	if !strings.Contains(apiURL, "/images/generations") {
		apiURL = apiURL + "/images/generations"
	}

	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Printf("[%s] HTTP错误: %v", p.Name, err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data []struct{ URL string `json:"url"` } `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Data) == 0 {
		log.Printf("[%s] 解析失败: %s", p.Name, string(body))
		return nil
	}

	imageURL := result.Data[0].URL
	return downloadAndSave(p, "siliconflow", imageURL)
}

// 阿里云百炼异步图片生成
func generateAliyunImage(p PlatformConfig, prompt string) *GenerateResult {
	client := &http.Client{Timeout: 30 * time.Second}

	// 步骤1: 创建任务
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": p.Model,
		"input": map[string]string{
			"prompt": prompt,
		},
		"parameters": map[string]interface{}{
			"size": fmt.Sprintf("%d*%d", cfg.ImageGen.Width, cfg.ImageGen.Height),
			"n":     1,
		},
	})

	req, _ := http.NewRequest("POST", "https://dashscope.aliyuncs.com/api/v1/services/aigc/text2image/image-synthesis", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[%s] 创建任务失败: %v", p.Name, err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var taskResp struct {
		Output struct {
			TaskID string `json:"task_id"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &taskResp); err != nil || taskResp.Output.TaskID == "" {
		log.Printf("[%s] 解析任务ID失败: %s", p.Name, string(body))
		return nil
	}

	taskID := taskResp.Output.TaskID
	log.Printf("[%s] 任务创建成功: %s", p.Name, taskID)

	// 步骤2: 轮询等待任务完成
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		time.Sleep(2 * time.Second)
		
		taskReq, _ := http.NewRequest("GET", "https://dashscope.aliyuncs.com/api/v1/tasks/"+taskID, nil)
		taskReq.Header.Set("Authorization", "Bearer "+p.APIKey)
		
		taskResp, err := client.Do(taskReq)
		if err != nil {
			continue
		}
		
		taskBody, _ := io.ReadAll(taskResp.Body)
		taskResp.Body.Close()
		
		var statusResp struct {
			Output struct {
				TaskStatus string `json:"task_status"`
				Results    []struct {
					URL string `json:"url"`
				} `json:"results"`
			} `json:"output"`
		}
		json.Unmarshal(taskBody, &statusResp)
		
		if statusResp.Output.TaskStatus == "SUCCEEDED" && len(statusResp.Output.Results) > 0 {
			return downloadAndSave(p, "aliyun", statusResp.Output.Results[0].URL)
		} else if statusResp.Output.TaskStatus == "FAILED" {
			log.Printf("[%s] 任务失败: %s", p.Name, string(taskBody))
			return nil
		}
	}

	log.Printf("[%s] 任务超时", p.Name)
	return nil
}

// 魔塔社区异步图片生成
func generateModelScopeImage(p PlatformConfig, prompt, size string) *GenerateResult {
	client := &http.Client{Timeout: 30 * time.Second}

	// 构建请求参数
	reqParams := map[string]interface{}{
		"model":  p.Model,
		"prompt": prompt,
	}
	// 支持 size 参数（如 "1920x1080" 或 "2048x2048"）
	if size != "" {
		reqParams["size"] = size
	}

	// 步骤1: 创建任务
	reqBody, _ := json.Marshal(reqParams)

	req, _ := http.NewRequest("POST", p.URL+"/v1/images/generations", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ModelScope-Async-Mode", "true")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[%s] 创建任务失败: %v", p.Name, err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var taskResp struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
	}
	json.Unmarshal(body, &taskResp)

	if taskResp.TaskID == "" {
		log.Printf("[%s] 解析任务ID失败: %s", p.Name, string(body))
		return nil
	}

	taskID := taskResp.TaskID
	log.Printf("[%s] 任务创建成功: %s", p.Name, taskID)

	// 步骤2: 轮询等待任务完成
	maxRetries := 60 // ModelScope 可能需要更长时间
	for i := 0; i < maxRetries; i++ {
		time.Sleep(3 * time.Second)

		taskReq, _ := http.NewRequest("GET", p.URL+"/v1/tasks/"+taskID, nil)
		taskReq.Header.Set("Authorization", "Bearer "+p.APIKey)
		taskReq.Header.Set("X-ModelScope-Task-Type", "image_generation")

		taskResp, err := client.Do(taskReq)
		if err != nil {
			continue
		}

		taskBody, _ := io.ReadAll(taskResp.Body)
		taskResp.Body.Close()

		var statusResp struct {
			TaskStatus  string   `json:"task_status"`
			OutputImages []string `json:"output_images"`
		}
		json.Unmarshal(taskBody, &statusResp)

		if statusResp.TaskStatus == "SUCCEED" && len(statusResp.OutputImages) > 0 {
			return downloadAndSave(p, "modelscope", statusResp.OutputImages[0])
		} else if statusResp.TaskStatus == "FAILED" {
			log.Printf("[%s] 任务失败: %s", p.Name, string(taskBody))
			return nil
		}
		log.Printf("[%s] 任务状态: %s", p.Name, statusResp.TaskStatus)
	}

	log.Printf("[%s] 任务超时", p.Name)
	return nil
}

// 下载并保存图片
func downloadAndSave(p PlatformConfig, platform, imageURL string) *GenerateResult {
	now := time.Now()
	dateDir := now.Format("2006-01-02")
	dir := filepath.Join(cfg.ImageGen.OutputDir, dateDir, platform)
	os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("%s.png", now.Format("150405"))
	path := filepath.Join(dir, filename)

	// 下载图片
	imgResp, err := http.Get(imageURL)
	if err != nil {
		log.Printf("[%s] 下载失败: %v", p.Name, err)
		return nil
	}
	defer imgResp.Body.Read(make([]byte, 0))
	data, _ := io.ReadAll(imgResp.Body)
	os.WriteFile(path, data, 0644)

	log.Printf("[%s] 生成成功: %s", p.Name, path)
	return &GenerateResult{
		Platform: p.Name,
		Model:    p.Model,
		Filename: filename,
		FilePath: path,
		Success:  true,
	}
}
