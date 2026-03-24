package v1

import (
	"net/http"
	"time"

	"claw-export-platform/api/middleware"
	"claw-export-platform/api/utils"
	"claw-export-platform/config"
	"claw-export-platform/models"
	"claw-export-platform/pkg/encryption"
	"claw-export-platform/pkg/queue"
	"claw-export-platform/services/export"
	"claw-export-platform/services/s3"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ExportHandler 导出API处理器
type ExportHandler struct {
	db        *gorm.DB
	queue     *queue.Queue
	encryptor *encryption.Encryptor
	logger    *zap.Logger
}

// NewExportHandler 创建导出API处理器
func NewExportHandler(db *gorm.DB, q *queue.Queue, encryptor *encryption.Encryptor, logger *zap.Logger) *ExportHandler {
	return &ExportHandler{
		db:        db,
		queue:     q,
		encryptor: encryptor,
		logger:    logger,
	}
}

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	TiDBConfigName string `json:"tidb_config_name" binding:"required"`
	S3ConfigName   string `json:"s3_config_name" binding:"required"`
	SqlText        string `json:"sql_text" binding:"required"`
	Filetype       string `json:"filetype"`
	Compress       string `json:"compress"`
	RetentionHours int    `json:"retention_hours"`
	TaskName       string `json:"task_name"`
	Priority       int    `json:"priority"`
}

// CreateTask 创建导出任务
func (h *ExportHandler) CreateTask(c *gin.Context) {
	tenantID := c.GetInt64(string(middleware.ContextKeyTenantID))

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败: "+err.Error())
		return
	}

	// 验证SQL安全性
	if err := export.ValidateSQL(req.SqlText); err != nil {
		utils.BadRequestWithData(c, "SQL语句包含非法关键字", gin.H{
			"field": "sql_text",
			"error": err.Error(),
		})
		return
	}

	// 设置默认值
	if req.Filetype == "" {
		req.Filetype = "sql"
	}
	if req.RetentionHours == 0 {
		req.RetentionHours = 168
	}
	if req.Priority == 0 {
		req.Priority = 5
	}

	// 查询TiDB配置
	var tidbConfig models.TiDBConfig
	if err := h.db.Where("tenant_id = ? AND name = ?", tenantID, req.TiDBConfigName).First(&tidbConfig).Error; err != nil {
		utils.NotFound(c, "TiDB配置不存在")
		return
	}

	// 查询S3配置
	var s3Config models.S3Config
	if err := h.db.Where("tenant_id = ? AND name = ?", tenantID, req.S3ConfigName).First(&s3Config).Error; err != nil {
		utils.NotFound(c, "S3配置不存在")
		return
	}

	// 检查配额
	var quota models.TenantQuota
	if err := h.db.Where("tenant_id = ?", tenantID).First(&quota).Error; err == nil {
		// 检查并发任务数
		var runningCount int64
		h.db.Model(&models.ExportTask{}).Where("tenant_id = ? AND status = ?", tenantID, models.TaskStatusRunning).Count(&runningCount)
		if runningCount >= int64(quota.MaxConcurrentTasks) {
			utils.QuotaExceeded(c, "并发任务数已达上限")
			return
		}

		// 检查每日任务数
		var todayCount int64
		today := time.Now().Format("2006-01-02")
		h.db.Model(&models.ExportTask{}).Where("tenant_id = ? AND DATE(created_at) = ?", tenantID, today).Count(&todayCount)
		if todayCount >= int64(quota.MaxDailyTasks) {
			utils.QuotaExceeded(c, "今日任务数已达上限")
			return
		}
	}

	// 创建任务
	task := &models.ExportTask{
		TenantID:       tenantID,
		TaskName:       req.TaskName,
		TiDBConfigID:   tidbConfig.ID,
		S3ConfigID:     s3Config.ID,
		SqlText:        req.SqlText,
		Filetype:       req.Filetype,
		Compress:       req.Compress,
		RetentionHours: req.RetentionHours,
		Priority:       req.Priority,
		Status:         models.TaskStatusPending,
		MaxRetries:     3,
	}

	if err := h.db.Create(task).Error; err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		utils.InternalError(c, "创建任务失败")
		return
	}

	// 加入队列
	if err := h.queue.Enqueue(c.Request.Context(), task); err != nil {
		h.logger.Error("failed to enqueue task", zap.Error(err))
		utils.InternalError(c, "任务入队失败")
		return
	}

	utils.Accepted(c, gin.H{
		"task_id":    task.ID,
		"status":     task.Status,
		"created_at": task.CreatedAt.Format(time.RFC3339),
	})
}

// GetTask 查询任务状态
func (h *ExportHandler) GetTask(c *gin.Context) {
	tenantID := c.GetInt64(string(middleware.ContextKeyTenantID))
	taskID := c.Param("task_id")

	var task models.ExportTask
	if err := h.db.Where("id = ? AND tenant_id = ?", taskID, tenantID).First(&task).Error; err != nil {
		utils.NotFound(c, "任务不存在")
		return
	}

	data := gin.H{
		"task_id":       task.ID,
		"task_name":     task.TaskName,
		"status":        task.Status,
		"file_url":      task.FileURL,
		"file_size":     task.FileSize,
		"row_count":     task.RowCount,
		"error_message": task.ErrorMessage,
	}

	if task.StartedAt != nil {
		data["started_at"] = task.StartedAt.Format(time.RFC3339)
	}
	if task.CompletedAt != nil {
		data["completed_at"] = task.CompletedAt.Format(time.RFC3339)
	}
	if task.ExpiresAt != nil {
		data["expires_at"] = task.ExpiresAt.Format(time.RFC3339)
	}

	utils.Success(c, data)
}

// CancelTask 取消任务
func (h *ExportHandler) CancelTask(c *gin.Context) {
	tenantID := c.GetInt64(string(middleware.ContextKeyTenantID))
	taskID := c.Param("task_id")

	var task models.ExportTask
	if err := h.db.Where("id = ? AND tenant_id = ?", taskID, tenantID).First(&task).Error; err != nil {
		utils.NotFound(c, "任务不存在")
		return
	}

	// 检查任务状态
	if task.Status == models.TaskStatusSuccess || task.Status == models.TaskStatusFailed || task.Status == models.TaskStatusCanceled {
		utils.Error(c, utils.CodeQuotaExceeded, "任务已完成，无法取消")
		return
	}

	// 更新任务状态
	now := time.Now()
	if err := h.db.Model(&task).Updates(map[string]interface{}{
		"status":        models.TaskStatusCanceled,
		"canceled_at":   now,
		"cancel_reason": "用户主动取消",
	}).Error; err != nil {
		utils.InternalError(c, "取消任务失败")
		return
	}

	utils.SuccessWithMessage(c, "任务已取消", gin.H{
		"task_id": task.ID,
		"status":  task.Status,
	})
}

// BatchQueryRequest 批量查询请求
type BatchQueryRequest struct {
	TaskIDs []string `json:"task_ids" binding:"required"`
}

// BatchQuery 批量查询任务状态
func (h *ExportHandler) BatchQuery(c *gin.Context) {
	tenantID := c.GetInt64(string(middleware.ContextKeyTenantID))

	var req BatchQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败")
		return
	}

	var tasks []models.ExportTask
	h.db.Where("id IN ? AND tenant_id = ?", req.TaskIDs, tenantID).Find(&tasks)

	results := make([]gin.H, len(tasks))
	for i, task := range tasks {
		result := gin.H{
			"task_id": task.ID,
			"status":  task.Status,
		}
		if task.FileURL != "" {
			result["file_url"] = task.FileURL
		}
		if task.ErrorMessage != "" {
			result["error_message"] = task.ErrorMessage
		}
		results[i] = result
	}

	utils.Success(c, results)
}

// GetFile 文件下载代理（使用预签名URL）
func (h *ExportHandler) GetFile(c *gin.Context) {
	tenantID := c.GetInt64(string(middleware.ContextKeyTenantID))
	taskID := c.Param("task_id")

	var task models.ExportTask
	if err := h.db.Where("id = ? AND tenant_id = ?", taskID, tenantID).First(&task).Error; err != nil {
		utils.NotFound(c, "任务不存在")
		return
	}

	if task.FileURL == "" {
		utils.NotFound(c, "文件不存在")
		return
	}

	// 检查是否过期
	if task.ExpiresAt != nil && task.ExpiresAt.Before(time.Now()) {
		utils.NotFound(c, "文件已过期")
		return
	}

	// 获取S3配置
	var s3Config models.S3Config
	if err := h.db.First(&s3Config, task.S3ConfigID).Error; err != nil {
		h.logger.Error("failed to get s3 config", zap.Error(err))
		utils.InternalError(c, "获取S3配置失败")
		return
	}

	// 解密SecretKey
	secretKey, err := h.encryptor.Decrypt(s3Config.SecretKeyEncrypted)
	if err != nil {
		h.logger.Error("failed to decrypt s3 secret key", zap.Error(err))
		utils.InternalError(c, "解密S3密钥失败")
		return
	}

	// 创建S3客户端
	s3Client, err := s3.NewClient(c.Request.Context(), s3.Config{
		Endpoint:   s3Config.Endpoint,
		AccessKey:  s3Config.AccessKey,
		SecretKey:  secretKey,
		Bucket:     s3Config.Bucket,
		Region:     s3Config.Region,
		PathPrefix: s3Config.PathPrefix,
	})
	if err != nil {
		h.logger.Error("failed to create s3 client", zap.Error(err))
		utils.InternalError(c, "创建S3客户端失败")
		return
	}

	// 计算预签名URL有效期（使用文件的剩余有效期，至少1小时）
	expiresIn := time.Hour
	if task.ExpiresAt != nil {
		remaining := time.Until(*task.ExpiresAt)
		if remaining > time.Hour {
			expiresIn = remaining
		} else if remaining > 0 {
			expiresIn = remaining
		}
	}

	// 生成预签名URL
	presignedURL, err := s3Client.GetPresignedURL(c.Request.Context(), task.FileURL, expiresIn)
	if err != nil {
		h.logger.Error("failed to generate presigned url", zap.Error(err))
		utils.InternalError(c, "生成下载链接失败")
		return
	}

	// 重定向到预签名URL
	c.Redirect(http.StatusFound, presignedURL)
}

// AdminHandler 管理API处理器
type AdminHandler struct {
	db        *gorm.DB
	encryptor *encryption.Encryptor
	logger    *zap.Logger
}

// NewAdminHandler 创建管理API处理器
func NewAdminHandler(db *gorm.DB, encryptor *encryption.Encryptor, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		db:        db,
		encryptor: encryptor,
		logger:    logger,
	}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 管理员登录
func (h *AdminHandler) Login(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(c, "参数验证失败")
			return
		}

		var admin models.Admin
		if err := h.db.Where("username = ?", req.Username).First(&admin).Error; err != nil {
			utils.Unauthorized(c, "用户名或密码错误")
			return
		}

		if admin.Status != 1 {
			utils.Forbidden(c, "账号已被禁用")
			return
		}

		// 验证密码
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
			utils.Unauthorized(c, "用户名或密码错误")
			return
		}

		// 生成Token
		token, err := middleware.GenerateToken(cfg, &admin)
		if err != nil {
			utils.InternalError(c, "生成Token失败")
			return
		}

		// 更新最后登录时间
		now := time.Now()
		h.db.Model(&admin).Update("last_login_at", now)

		utils.Success(c, gin.H{
			"token":      token,
			"expires_in": cfg.Security.TokenExpireHour * 3600,
			"user": gin.H{
				"id":       admin.ID,
				"username": admin.Username,
				"role":     admin.Role,
			},
		})
	}
}

// RefreshToken 刷新Token
func (h *AdminHandler) RefreshToken(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从上下文获取管理员信息
		adminID := c.GetInt64(string(middleware.ContextKeyAdminID))

		var admin models.Admin
		if err := h.db.First(&admin, adminID).Error; err != nil {
			utils.Unauthorized(c, "用户不存在")
			return
		}

		if admin.Status != 1 {
			utils.Forbidden(c, "账号已被禁用")
			return
		}

		// 生成新Token
		token, err := middleware.GenerateToken(cfg, &admin)
		if err != nil {
			utils.InternalError(c, "生成Token失败")
			return
		}

		utils.Success(c, gin.H{
			"token":      token,
			"expires_in": cfg.Security.TokenExpireHour * 3600,
		})
	}
}

// Logout 登出
func (h *AdminHandler) Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		// JWT Token 是无状态的，客户端删除 Token 即可
		// 如果需要实现 Token 黑名单，可以在这里将 Token 加入 Redis 黑名单
		utils.SuccessWithMessage(c, "登出成功", nil)
	}
}

// GetProfile 获取当前用户信息
func (h *AdminHandler) GetProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, ok := middleware.GetAdminFromContext(c)
		if !ok {
			utils.Unauthorized(c, "未登录")
			return
		}

		utils.Success(c, gin.H{
			"id":            admin.ID,
			"username":      admin.Username,
			"email":         admin.Email,
			"role":          admin.Role,
			"last_login_at": admin.LastLoginAt,
			"created_at":    admin.CreatedAt,
		})
	}
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// ChangePassword 修改密码
func (h *AdminHandler) ChangePassword() gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, ok := middleware.GetAdminFromContext(c)
		if !ok {
			utils.Unauthorized(c, "未登录")
			return
		}

		var req ChangePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.BadRequest(c, "参数验证失败: "+err.Error())
			return
		}

		// 验证旧密码
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.OldPassword)); err != nil {
			utils.BadRequest(c, "旧密码错误")
			return
		}

		// 生成新密码哈希
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			utils.InternalError(c, "密码加密失败")
			return
		}

		// 更新密码
		if err := h.db.Model(admin).Update("password_hash", string(newHash)).Error; err != nil {
			utils.InternalError(c, "更新密码失败")
			return
		}

		utils.SuccessWithMessage(c, "密码修改成功", nil)
	}
}
