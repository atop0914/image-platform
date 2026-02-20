package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局配置
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	ImageGen   ImageGenConfig  `yaml:"imageGen"`
	Platforms  PlatformConfigs `yaml:"platforms"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port string `yaml:"port"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
}

// ImageGenConfig 图片生成配置
type ImageGenConfig struct {
	OutputDir  string `yaml:"outputDir"`
	LogDir     string `yaml:"logDir"`
	Width      int    `yaml:"width"`
	Height     int    `yaml:"height"`
	MaxRetries int    `yaml:"maxRetries"`
	RetryDelay int    `yaml:"retryDelay"`
	Timeout    int    `yaml:"timeout"`
	MaxWorkers int    `yaml:"maxWorkers"`
}

// PlatformConfigs 平台配置
type PlatformConfigs map[string]PlatformConfig

// PlatformConfig 单个平台配置
type PlatformConfig struct {
	Name        string `yaml:"name"`
	EnvKey      string `yaml:"envKey"`
	APIKey      string `yaml:"apiKey"`
	URL         string `yaml:"url"`
	Model       string `yaml:"model"`
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description"`
}

// Load 加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if cfg.ImageGen.OutputDir == "" {
		cfg.ImageGen.OutputDir = os.ExpandEnv("$HOME/generated_images")
	}
	if cfg.ImageGen.LogDir == "" {
		cfg.ImageGen.LogDir = os.ExpandEnv("$HOME/generated_images/logs")
	}
	if cfg.ImageGen.Width == 0 {
		cfg.ImageGen.Width = 1024
	}
	if cfg.ImageGen.Height == 0 {
		cfg.ImageGen.Height = 2048
	}
	if cfg.ImageGen.MaxRetries == 0 {
		cfg.ImageGen.MaxRetries = 3
	}
	if cfg.ImageGen.RetryDelay == 0 {
		cfg.ImageGen.RetryDelay = 3
	}
	if cfg.ImageGen.Timeout == 0 {
		cfg.ImageGen.Timeout = 180
	}
	if cfg.ImageGen.MaxWorkers == 0 {
		cfg.ImageGen.MaxWorkers = 5
	}

	// 从环境变量加载 API Key
	cfg.loadAPIKeys()

	return &cfg, nil
}

// loadAPIKeys 从环境变量加载 API Key
func (c *Config) loadAPIKeys() {
	for key, cfg := range c.Platforms {
		apiKey := os.Getenv(cfg.EnvKey)
		if apiKey != "" {
			cfg.APIKey = apiKey
			cfg.Enabled = true
		}
		c.Platforms[key] = cfg
	}
}

// GetEnabledPlatforms 获取已启用的平台
func (c *Config) GetEnabledPlatforms() map[string]PlatformConfig {
	enabled := make(map[string]PlatformConfig)
	for key, cfg := range c.Platforms {
		if cfg.Enabled && cfg.APIKey != "" {
			enabled[key] = cfg
		}
	}
	return enabled
}
