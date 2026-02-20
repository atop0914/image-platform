package publisher

import (
	"context"
	"fmt"
	"log"
)

// Platform 平台接口
type Platform interface {
	Name() string
	Publish(ctx context.Context, imgPath, title, content string) (string, error)
	Type() PlatformType
}

// PlatformType 平台类型
type PlatformType string

const (
	PlatformXiaohongshu PlatformType = "xiaohongshu"
	PlatformDouyin      PlatformType = "douyin"
	PlatformBilibili    PlatformType = "bilibili"
	PlatformTwitter     PlatformType = "twitter"
	PlatformCustom     PlatformType = "custom"
)

// Manager 发布管理器
type Manager struct {
	platforms map[PlatformType]Platform
}

// New 创建发布管理器
func New() *Manager {
	return &Manager{
		platforms: make(map[PlatformType]Platform),
	}
}

// Register 注册平台
func (m *Manager) Register(p Platform) {
	m.platforms[p.Type()] = p
	log.Printf("📤 已注册发布平台: %s", p.Name())
}

// Get 获取平台
func (m *Manager) Get(t PlatformType) Platform {
	return m.platforms[t]
}

// List 列出所有平台
func (m *Manager) List() []Platform {
	result := make([]Platform, 0, len(m.platforms))
	for _, p := range m.platforms {
		result = append(result, p)
	}
	return result
}

// Publish 发布到指定平台
func (m *Manager) Publish(platformType PlatformType, ctx context.Context, imgPath, title, content string) (string, error) {
	p, ok := m.platforms[platformType]
	if !ok {
		return "", fmt.Errorf("未支持的平台: %s", platformType)
	}
	return p.Publish(ctx, imgPath, title, content)
}

// PublishAll 发布到所有平台
func (m *Manager) PublishAll(ctx context.Context, imgPath, title, content string) map[string]string {
	results := make(map[string]string)
	for _, p := range m.platforms {
		url, err := p.Publish(ctx, imgPath, title, content)
		if err != nil {
			results[p.Name()] = "失败: " + err.Error()
		} else {
			results[p.Name()] = url
		}
	}
	return results
}
