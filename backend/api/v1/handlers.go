package v1

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
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
		FileURLs:       "[]",
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

	var s3Client s3.StorageClient
	if task.Status == models.TaskStatusSuccess {
		s3Client = h.buildTaskS3Client(c, task.S3ConfigID)
	}

	files, fileURL := h.buildTaskFilesResponse(c, task, s3Client)
	data := gin.H{
		"task_id":       task.ID,
		"task_name":     task.TaskName,
		"status":        task.Status,
		"file_url":      fileURL,
		"files":         files,
		"file_count":    len(files),
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

	s3Clients := make(map[int64]s3.StorageClient)
	for _, task := range tasks {
		if task.Status != models.TaskStatusSuccess || task.S3ConfigID <= 0 {
			continue
		}
		if _, ok := s3Clients[task.S3ConfigID]; ok {
			continue
		}
		if client := h.buildTaskS3Client(c, task.S3ConfigID); client != nil {
			s3Clients[task.S3ConfigID] = client
		}
	}

	results := make([]gin.H, len(tasks))
	for i, task := range tasks {
		result := gin.H{
			"task_id": task.ID,
			"status":  task.Status,
		}

		files, fileURL := h.buildTaskFilesResponse(c, task, s3Clients[task.S3ConfigID])
		if fileURL != "" {
			result["file_url"] = fileURL
		}
		if len(files) > 0 {
			result["files"] = files
			result["file_count"] = len(files)
		}
		if task.ErrorMessage != "" {
			result["error_message"] = task.ErrorMessage
		}
		results[i] = result
	}

	utils.Success(c, results)
}

// GetFile 文件下载代理（单文件重定向；多文件返回清单）
func (h *ExportHandler) GetFile(c *gin.Context) {
	tenantID := c.GetInt64(string(middleware.ContextKeyTenantID))
	taskID := c.Param("task_id")

	var task models.ExportTask
	if err := h.db.Where("id = ? AND tenant_id = ?", taskID, tenantID).First(&task).Error; err != nil {
		utils.NotFound(c, "任务不存在")
		return
	}

	files := parseTaskFiles(task)
	if len(files) == 0 {
		utils.NotFound(c, "文件不存在")
		return
	}

	// 检查是否过期
	if task.ExpiresAt != nil && task.ExpiresAt.Before(time.Now()) {
		utils.NotFound(c, "文件已过期")
		return
	}

	s3Client := h.buildTaskS3Client(c, task.S3ConfigID)
	if s3Client == nil {
		utils.InternalError(c, "创建S3客户端失败")
		return
	}

	expiresIn := calcPresignedExpire(task)

	if len(files) == 1 {
		presignedURL, err := s3Client.GetPresignedURL(c.Request.Context(), files[0].Path, expiresIn)
		if err != nil {
			h.logger.Error("failed to generate presigned url", zap.Error(err))
			utils.InternalError(c, "生成下载链接失败")
			return
		}
		c.Redirect(http.StatusFound, presignedURL)
		return
	}

	respFiles := make([]gin.H, 0, len(files))
	for i, file := range files {
		url := file.Path
		if presignedURL, err := s3Client.GetPresignedURL(c.Request.Context(), file.Path, expiresIn); err == nil {
			url = presignedURL
		}
		respFiles = append(respFiles, gin.H{
			"index": i,
			"name":  fileNameOf(file),
			"path":  file.Path,
			"url":   url,
			"size":  file.Size,
		})
	}

	utils.Success(c, gin.H{
		"task_id":    task.ID,
		"status":     task.Status,
		"file_count": len(respFiles),
		"files":      respFiles,
	})
}

type taskFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func parseTaskFiles(task models.ExportTask) []taskFile {
	if strings.TrimSpace(task.FileURLs) != "" {
		var files []taskFile
		if err := json.Unmarshal([]byte(task.FileURLs), &files); err == nil && len(files) > 0 {
			return files
		}
	}
	if strings.TrimSpace(task.FileURL) == "" {
		return nil
	}
	return []taskFile{{
		Path: task.FileURL,
		Name: filepath.Base(task.FileURL),
		Size: task.FileSize,
	}}
}

func fileNameOf(file taskFile) string {
	if strings.TrimSpace(file.Name) != "" {
		return file.Name
	}
	return filepath.Base(file.Path)
}

func calcPresignedExpire(task models.ExportTask) time.Duration {
	expiresIn := time.Hour
	if task.ExpiresAt == nil {
		return expiresIn
	}
	remaining := time.Until(*task.ExpiresAt)
	if remaining > time.Hour {
		return remaining
	}
	if remaining > 0 {
		return remaining
	}
	return expiresIn
}

func (h *ExportHandler) buildTaskS3Client(c *gin.Context, s3ConfigID int64) s3.StorageClient {
	var s3Config models.S3Config
	if err := h.db.First(&s3Config, s3ConfigID).Error; err != nil {
		h.logger.Warn("failed to get s3 config", zap.Int64("s3_config_id", s3ConfigID), zap.Error(err))
		return nil
	}

	secretKey, err := h.encryptor.Decrypt(s3Config.SecretKeyEncrypted)
	if err != nil {
		h.logger.Warn("failed to decrypt s3 secret key", zap.Int64("s3_config_id", s3ConfigID), zap.Error(err))
		return nil
	}

	s3Client, err := s3.NewStorageClient(c.Request.Context(), s3.Config{
		Provider:   string(s3Config.Provider),
		Endpoint:   s3Config.Endpoint,
		AccessKey:  s3Config.AccessKey,
		SecretKey:  secretKey,
		Bucket:     s3Config.Bucket,
		Region:     s3Config.Region,
		PathPrefix: s3Config.PathPrefix,
	})
	if err != nil {
		h.logger.Warn("failed to create s3 client", zap.Int64("s3_config_id", s3ConfigID), zap.Error(err))
		return nil
	}
	return s3Client
}

func (h *ExportHandler) buildTaskFilesResponse(c *gin.Context, task models.ExportTask, s3Client s3.StorageClient) ([]gin.H, string) {
	taskFiles := parseTaskFiles(task)
	if len(taskFiles) == 0 {
		return nil, ""
	}

	expiresIn := calcPresignedExpire(task)
	respFiles := make([]gin.H, 0, len(taskFiles))
	primaryURL := ""

	for i, file := range taskFiles {
		url := file.Path
		if s3Client != nil && task.Status == models.TaskStatusSuccess {
			if presignedURL, err := s3Client.GetPresignedURL(c.Request.Context(), file.Path, expiresIn); err == nil {
				url = presignedURL
			}
		}

		if i == 0 {
			primaryURL = url
		}
		respFiles = append(respFiles, gin.H{
			"index": i,
			"name":  fileNameOf(file),
			"path":  file.Path,
			"url":   url,
			"size":  file.Size,
		})
	}

	return respFiles, primaryURL
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
