package main

import (
	"bytes"
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
)

// ========== 配置 ==========
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	ImageGen   ImageGenConfig  `yaml:"imageGen"`
	Platforms  PlatformConfigs `yaml:"platforms"`
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
	Name    string `yaml:"name"`
	EnvKey  string `yaml:"envKey"`
	APIKey  string `yaml:"apiKey"`
	URL     string `yaml:"url"`
	Model   string `yaml:"model"`
	Enabled bool   `yaml:"enabled"`
}

// ========== 数据模型 ==========
type ImageRecord struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"size:255;not null" json:"name"`
	Date        string     `gorm:"size:20;not null" json:"date"`
	Path        string     `gorm:"size:512;not null" json:"path"`
	Platform    string     `gorm:"size:50;not null" json:"platform"`      // 平台名称
	Model       string     `gorm:"size:100;not null" json:"model"`       // 模型名称
	Prompt      string     `gorm:"size:1000" json:"prompt"`              // 提示词
	GeneratedAt time.Time  `gorm:"not null" json:"generated_at"`         // 生成时间
	Status      string     `gorm:"size:20;default:'pending'" json:"status"`
	Note        string     `gorm:"type:text" json:"note"`
	ModeratedAt *time.Time `json:"moderated_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (ImageRecord) TableName() string {
	return "images"
}

// ========== 全局变量 ==========
var db *gorm.DB
var cfg *Config

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

	for key, p := range cfg.Platforms {
		if p.Enabled && p.APIKey != "" {
			log.Printf("已启用平台: %s - %s", key, p.Name)
		}
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.LoadHTMLGlob("web/templates/*")
	r.Static("/static", "./web/static")

	r.GET("/", index)
	r.GET("/add", addPage)
	r.GET("/moderate/:id", moderatePage)
	r.GET("/records", recordsPage)

	r.POST("/api/generate", handleGenerate)
	r.GET("/api/images", listImages)
	r.POST("/api/moderate", moderateImage)
	r.GET("/api/records", listRecords)
	r.DELETE("/api/images/:id", deleteImage)
	r.GET("/api/report", dailyReport)

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

// ========== API 处理 ==========
func handleGenerate(c *gin.Context) {
	var req struct {
		Prompt   string `json:"prompt" binding:"required"`
		Platform string `json:"platform"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	platforms := getEnabledPlatforms()
	var result *GenerateResult

	if req.Platform != "" {
		result = generateImage(req.Platform, req.Prompt)
	} else {
		for key, p := range platforms {
			r := generateImage(key, req.Prompt)
			if r != nil {
				result = r
				break
			}
			log.Printf("[%s] 生成失败", p.Name)
		}
	}

	if result == nil {
		c.JSON(500, gin.H{"error": "所有平台生成失败"})
		return
	}

	// 自动投递审核
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

// ========== 图片生成 ==========
type GenerateResult struct {
	Platform  string
	Model     string
	Filename  string
	FilePath  string
	Success   bool
}

func generateImage(platform, prompt string) *GenerateResult {
	p, ok := cfg.Platforms[platform]
	if !ok || !p.Enabled {
		return nil
	}

	client := &http.Client{Timeout: 60 * time.Second}
	size := fmt.Sprintf("%dx%d", cfg.ImageGen.Width, cfg.ImageGen.Height)

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
	
	// 目录结构: outputDir/日期/平台/时间戳.png
	now := time.Now()
	dateDir := now.Format("2006-01-02")
	platformDir := platform
	
	dir := filepath.Join(cfg.ImageGen.OutputDir, dateDir, platformDir)
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
