package moderation

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ImageRecord 图片记录
type ImageRecord struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"size:255;not null" json:"name"`
	Date        string     `gorm:"size:20;not null" json:"date"`
	Path        string     `gorm:"size:512;not null" json:"path"`
	Status      string     `gorm:"size:20;default:'pending'" json:"status"`
	Note        string     `gorm:"type:text" json:"note"`
	ModeratedAt *time.Time `json:"moderated_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (ImageRecord) TableName() string {
	return "images"
}

// Repository 数据访问层
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(record *ImageRecord) error {
	return r.db.Create(record).Error
}

func (r *Repository) Update(id uint, status, note string) error {
	now := time.Now()
	return r.db.Model(&ImageRecord{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       status,
			"note":         note,
			"moderated_at": now,
		}).Error
}

func (r *Repository) Delete(id uint) error {
	return r.db.Delete(&ImageRecord{}, id).Error
}

func (r *Repository) FindByID(id uint) (*ImageRecord, error) {
	var record ImageRecord
	err := r.db.Where("id = ?", id).First(&record).Error
	return &record, err
}

func (r *Repository) ListByStatus(status string, limit, offset int) ([]ImageRecord, int64, error) {
	var records []ImageRecord
	var total int64

	query := r.db.Model(&ImageRecord{})
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&records).Error
	return records, total, err
}

func (r *Repository) ListAll(limit, offset int) ([]ImageRecord, int64, error) {
	return r.ListByStatus("", limit, offset)
}

func (r *Repository) ListByDate(date string, limit, offset int) ([]ImageRecord, int64, error) {
	var records []ImageRecord
	var total int64

	query := r.db.Model(&ImageRecord{}).Where("date = ?", date)
	query.Count(&total)
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&records).Error
	return records, total, err
}

// Handler HTTP 处理器
type Handler struct {
	repo *Repository
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{repo: NewRepository(db)}
}

// AddImage 添加图片
func (h *Handler) AddImage(record *ImageRecord) error {
	return h.repo.Create(record)
}

// Index 首页
func (h *Handler) Index(c *gin.Context) {
	records, total, _ := h.repo.ListByStatus("pending", 100, 0)
	approved, _, _ := h.repo.ListByStatus("approved", 100, 0)
	rejected, _, _ := h.repo.ListByStatus("rejected", 100, 0)

	c.HTML(http.StatusOK, "index.html", gin.H{
		"records":      records,
		"total":        total,
		"approved":     len(approved),
		"rejected":     len(rejected),
		"pendingCount": len(records),
	})
}

// AddPage 添加页面
func (h *Handler) AddPage(c *gin.Context) {
	c.HTML(http.StatusOK, "add.html", nil)
}

// ModeratePage 审核页面
func (h *Handler) ModeratePage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	record, err := h.repo.FindByID(uint(id))
	if err != nil {
		c.String(http.StatusNotFound, "Image not found")
		return
	}
	c.HTML(http.StatusOK, "moderate.html", gin.H{"record": record})
}

// RecordsPage 记录页面
func (h *Handler) RecordsPage(c *gin.Context) {
	records, total, _ := h.repo.ListAll(100, 0)
	c.HTML(http.StatusOK, "records.html", gin.H{
		"records": records,
		"total":   total,
	})
}

// ListImages API - 图片列表
func (h *Handler) ListImages(c *gin.Context) {
	status := c.DefaultQuery("status", "all")
	records, total, _ := h.repo.ListByStatus(status, 100, 0)
	c.JSON(200, gin.H{"records": records, "total": total})
}

// Moderate API - 提交审核
func (h *Handler) Moderate(c *gin.Context) {
	var req struct {
		ID     uint   `json:"id" binding:"required"`
		Status string `json:"status" binding:"required"`
		Note   string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.Update(req.ID, req.Status, req.Note); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "success"})
}

// ListRecords API - 审核记录
func (h *Handler) ListRecords(c *gin.Context) {
	records, total, _ := h.repo.ListAll(100, 0)
	c.JSON(200, gin.H{"records": records, "total": total})
}

// DeleteImage API - 删除记录
func (h *Handler) DeleteImage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := h.repo.Delete(uint(id)); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "success"})
}

// DailyReport API - 当天报告
func (h *Handler) DailyReport(c *gin.Context) {
	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	records, total, _ := h.repo.ListByDate(date, 1000, 0)

	approved, rejected, pending := 0, 0, 0
	for _, r := range records {
		switch r.Status {
		case "approved":
			approved++
		case "rejected":
			rejected++
		default:
			pending++
		}
	}

	type ImageDetail struct {
		ID          uint   `json:"id"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		Date        string `json:"date"`
		Status      string `json:"status"`
		Note        string `json:"note"`
		ModeratedAt string `json:"moderated_at"`
		CreatedAt   string `json:"created_at"`
	}

	details := make([]ImageDetail, len(records))
	for i, r := range records {
		moderatedAt := ""
		if r.ModeratedAt != nil {
			moderatedAt = r.ModeratedAt.Format("2006-01-02 15:04:05")
		}
		details[i] = ImageDetail{
			ID:          r.ID,
			Name:        r.Name,
			Path:        r.Path,
			Date:        r.Date,
			Status:      r.Status,
			Note:        r.Note,
			ModeratedAt: moderatedAt,
			CreatedAt:   r.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(200, gin.H{
		"date":     date,
		"total":    total,
		"approved": approved,
		"rejected": rejected,
		"pending":  pending,
		"images":   details,
	})
}

// 导入 gin
func init() {}
