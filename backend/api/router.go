package api

import (
	"encoding/json"
	"strconv"
	"time"

	"claw-export-platform/api/middleware"
	"claw-export-platform/api/utils"
	v1 "claw-export-platform/api/v1"
	"claw-export-platform/config"
	"claw-export-platform/models"
	"claw-export-platform/pkg/encryption"
	"claw-export-platform/pkg/queue"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Router 路由配置
type Router struct {
	db        *gorm.DB
	queue     *queue.Queue
	encryptor *encryption.Encryptor
	cfg       *config.Config
	logger    *zap.Logger
}

// NewRouter 创建路由
func NewRouter(db *gorm.DB, q *queue.Queue, encryptor *encryption.Encryptor, cfg *config.Config, logger *zap.Logger) *Router {
	return &Router{
		db:        db,
		queue:     q,
		encryptor: encryptor,
		cfg:       cfg,
		logger:    logger,
	}
}

// Setup 设置路由
func (r *Router) Setup(engine *gin.Engine) {
	// 全局中间件
	engine.Use(middleware.RequestID())
	engine.Use(middleware.Logger(r.logger))
	engine.Use(middleware.Recovery(r.logger))
	engine.Use(middleware.CORS())

	// 健康检查
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "healthy",
			"version":   "1.0.0",
			"timestamp": gin.H{},
		})
	})

	engine.GET("/health/ready", func(c *gin.Context) {
		// 检查数据库连接
		sqlDB, err := r.db.DB()
		if err != nil {
			c.JSON(503, gin.H{"status": "not ready", "error": "database unavailable"})
			return
		}
		if err := sqlDB.Ping(); err != nil {
			c.JSON(503, gin.H{"status": "not ready", "error": "database ping failed"})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})

	// API v1
	v1Group := engine.Group("/api/v1")

	// 创建处理器
	exportHandler := v1.NewExportHandler(r.db, r.queue, r.encryptor, r.logger.Named("export"))
	adminHandler := v1.NewAdminHandler(r.db, r.encryptor, r.logger.Named("admin"))

	// 开放API（需要API Key认证）
	exportGroup := v1Group.Group("/export")
	exportGroup.Use(middleware.APIKeyAuth(r.db, r.encryptor, r.logger))
	{
		exportGroup.POST("/tasks", exportHandler.CreateTask)
		exportGroup.GET("/tasks/:task_id", exportHandler.GetTask)
		exportGroup.DELETE("/tasks/:task_id", exportHandler.CancelTask)
		exportGroup.POST("/tasks/batch", exportHandler.BatchQuery)
		exportGroup.GET("/files/:task_id", exportHandler.GetFile)
	}

	// 管理API（需要JWT认证）
	adminGroup := v1Group.Group("/admin")
	adminGroup.POST("/auth/login", adminHandler.Login(r.cfg))

	// 需要认证的管理API
	adminAuthGroup := adminGroup.Group("")
	adminAuthGroup.Use(middleware.JWTAuth(r.cfg, r.db, r.logger))
	{
		// 认证相关
		adminAuthGroup.POST("/auth/refresh", adminHandler.RefreshToken(r.cfg))
		adminAuthGroup.POST("/auth/logout", adminHandler.Logout())
		adminAuthGroup.GET("/auth/profile", adminHandler.GetProfile())
		adminAuthGroup.PUT("/auth/password", adminHandler.ChangePassword())
		// 租户管理
		adminAuthGroup.GET("/tenants", r.listTenants)
		adminAuthGroup.POST("/tenants", r.createTenant)
		adminAuthGroup.GET("/tenants/:id", r.getTenant)
		adminAuthGroup.PUT("/tenants/:id", r.updateTenant)
		adminAuthGroup.DELETE("/tenants/:id", r.deleteTenant)

		// 任务管理
		adminAuthGroup.GET("/tasks", r.listTasks)
		adminAuthGroup.GET("/tasks/:id", r.getTask)
		adminAuthGroup.GET("/tasks/:id/logs", r.getTaskLogs)

		// TiDB配置管理
		adminAuthGroup.GET("/tidb-configs", r.listTiDBConfigs)
		adminAuthGroup.POST("/tidb-configs", r.createTiDBConfig)
		adminAuthGroup.PUT("/tidb-configs/:id", r.updateTiDBConfig)
		adminAuthGroup.DELETE("/tidb-configs/:id", r.deleteTiDBConfig)

		// S3配置管理
		adminAuthGroup.GET("/s3-configs", r.listS3Configs)
		adminAuthGroup.POST("/s3-configs", r.createS3Config)
		adminAuthGroup.PUT("/s3-configs/:id", r.updateS3Config)
		adminAuthGroup.DELETE("/s3-configs/:id", r.deleteS3Config)

		// 审计日志
		adminAuthGroup.GET("/audit-logs", r.listAuditLogs)
	}
}

// 以下是管理API的实现

// listTenants 租户列表
func (r *Router) listTenants(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	var tenants []models.Tenant

	// 统计总数
	if err := r.db.Model(&models.Tenant{}).Count(&total).Error; err != nil {
		r.logger.Error("failed to count tenants", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := r.db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&tenants).Error; err != nil {
		r.logger.Error("failed to list tenants", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 构建响应（不返回敏感字段）
	items := make([]gin.H, len(tenants))
	for i, t := range tenants {
		items[i] = gin.H{
			"tenant_id":  t.ID,
			"name":       t.Name,
			"api_key":    t.APIKey,
			"status":     t.Status,
			"created_at": t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"items":     items,
		},
	})
}

type createTenantRequest struct {
	Name   string `json:"name" binding:"required"`
	Status int8   `json:"status"`
}

// createTenant 创建租户
func (r *Router) createTenant(c *gin.Context) {
	var req createTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败: "+err.Error())
		return
	}

	// 生成 API Key 和 API Secret
	apiKey := generateAPIKey()
	apiSecret := generateAPISecret()

	// 加密 API Secret
	apiSecretEncrypted, err := r.encryptor.Encrypt(apiSecret)
	if err != nil {
		r.logger.Error("failed to encrypt api secret", zap.Error(err))
		utils.InternalError(c, "加密密钥失败")
		return
	}

	// 设置默认状态
	if req.Status == 0 {
		req.Status = 1
	}

	// 创建租户
	tenant := &models.Tenant{
		Name:               req.Name,
		APIKey:             apiKey,
		APISecretEncrypted: apiSecretEncrypted,
		Status:             req.Status,
	}

	if err := r.db.Create(tenant).Error; err != nil {
		r.logger.Error("failed to create tenant", zap.Error(err))
		utils.InternalError(c, "创建租户失败")
		return
	}

	// 创建默认配额
	quota := &models.TenantQuota{
		TenantID:           tenant.ID,
		MaxConcurrentTasks: 5,
		MaxDailyTasks:      100,
		MaxDailySizeGB:     50,
		MaxRetentionHours:  720,
	}
	if err := r.db.Create(quota).Error; err != nil {
		r.logger.Error("failed to create tenant quota", zap.Error(err))
		// 不回滚租户创建，只记录错误
	}

	// 记录审计日志
	r.recordAuditLog(c, "create", "tenant", tenant.ID, gin.H{"name": req.Name}, "success")

	c.JSON(201, gin.H{
		"code":    0,
		"message": "租户创建成功",
		"data": gin.H{
			"tenant_id":  tenant.ID,
			"api_key":    apiKey,
			"api_secret": apiSecret, // 只在创建时返回一次
			"name":       tenant.Name,
			"status":     tenant.Status,
			"created_at": tenant.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	})
}

// getTenant 获取租户详情
func (r *Router) getTenant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var tenant models.Tenant
	if err := r.db.First(&tenant, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "租户不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	// 获取配额信息
	var quota models.TenantQuota
	r.db.Where("tenant_id = ?", id).First(&quota)

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"tenant_id":  tenant.ID,
			"name":       tenant.Name,
			"api_key":    tenant.APIKey,
			"status":     tenant.Status,
			"created_at": tenant.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"updated_at": tenant.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"quota": gin.H{
				"max_concurrent_tasks": quota.MaxConcurrentTasks,
				"max_daily_tasks":      quota.MaxDailyTasks,
				"max_daily_size_gb":    quota.MaxDailySizeGB,
				"max_retention_hours":  quota.MaxRetentionHours,
			},
		},
	})
}

type updateTenantRequest struct {
	Name   string `json:"name"`
	Status int8   `json:"status"`
}

// updateTenant 更新租户
func (r *Router) updateTenant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var tenant models.Tenant
	if err := r.db.First(&tenant, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "租户不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	var req updateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Status != 0 {
		updates["status"] = req.Status
	}

	if len(updates) > 0 {
		if err := r.db.Model(&tenant).Updates(updates).Error; err != nil {
			r.logger.Error("failed to update tenant", zap.Error(err))
			utils.InternalError(c, "更新失败")
			return
		}
	}

	// 记录审计日志
	r.recordAuditLog(c, "update", "tenant", id, updates, "success")

	c.JSON(200, gin.H{"code": 0, "message": "更新成功"})
}

// deleteTenant 删除租户（软删除）
func (r *Router) deleteTenant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	result := r.db.Delete(&models.Tenant{}, id)
	if result.Error != nil {
		r.logger.Error("failed to delete tenant", zap.Error(result.Error))
		utils.InternalError(c, "删除失败")
		return
	}
	if result.RowsAffected == 0 {
		utils.NotFound(c, "租户不存在")
		return
	}

	// 记录审计日志
	r.recordAuditLog(c, "delete", "tenant", id, nil, "success")

	c.JSON(200, gin.H{"code": 0, "message": "删除成功"})
}

// generateAPIKey 生成API Key
func generateAPIKey() string {
	return "sk_live_" + randomString(32)
}

// generateAPISecret 生成API Secret
func generateAPISecret() string {
	return "sk_secret_" + randomString(64)
}

// randomString 生成随机字符串
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[randomInt(len(charset))]
	}
	return string(b)
}

func randomInt(max int) int {
	// 简单实现，生产环境应使用 crypto/rand
	return int(uint64(time.Now().UnixNano()) % uint64(max))
}

func (r *Router) listTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 过滤条件
	status := c.Query("status")
	tenantID := c.Query("tenant_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := r.db.Model(&models.ExportTask{}).Preload("Tenant")

	// 应用过滤条件
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate+" 00:00:00")
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate+" 23:59:59")
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		r.logger.Error("failed to count tasks", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 分页查询
	var tasks []models.ExportTask
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		r.logger.Error("failed to list tasks", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 构建响应
	items := make([]gin.H, len(tasks))
	for i, task := range tasks {
		item := gin.H{
			"task_id":     task.ID,
			"task_name":   task.TaskName,
			"tenant_id":   task.TenantID,
			"status":      task.Status,
			"file_size":   task.FileSize,
			"row_count":   task.RowCount,
			"retry_count": task.RetryCount,
			"created_at":  task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		// 添加租户名称
		if task.Tenant.ID != 0 {
			item["tenant_name"] = task.Tenant.Name
		}

		if task.StartedAt != nil {
			item["started_at"] = task.StartedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if task.CompletedAt != nil {
			item["completed_at"] = task.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if task.ErrorMessage != "" {
			item["error_message"] = task.ErrorMessage
		}

		items[i] = item
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"items":     items,
		},
	})
}

func (r *Router) getTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var task models.ExportTask
	if err := r.db.Preload("Tenant").First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "任务不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	// 获取TiDB配置名称
	var tidbConfig models.TiDBConfig
	r.db.Select("name").First(&tidbConfig, task.TiDBConfigID)

	// 获取S3配置名称
	var s3Config models.S3Config
	r.db.Select("name").First(&s3Config, task.S3ConfigID)

	data := gin.H{
		"task_id":            task.ID,
		"task_name":          task.TaskName,
		"tenant_id":          task.TenantID,
		"tenant_name":        task.Tenant.Name,
		"tidb_config_name":   tidbConfig.Name,
		"s3_config_name":     s3Config.Name,
		"filetype":           task.Filetype,
		"compress":           task.Compress,
		"retention_hours":    task.RetentionHours,
		"priority":           task.Priority,
		"status":             task.Status,
		"file_url":           task.FileURL,
		"file_size":          task.FileSize,
		"row_count":          task.RowCount,
		"retry_count":        task.RetryCount,
		"max_retries":        task.MaxRetries,
		"error_message":      task.ErrorMessage,
		"cancel_reason":      task.CancelReason,
		"created_at":         task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":         task.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// SQL脱敏（只显示前100字符）
	if len(task.SqlText) > 100 {
		data["sql_text_preview"] = task.SqlText[:100] + "..."
	} else {
		data["sql_text_preview"] = task.SqlText
	}

	if task.StartedAt != nil {
		data["started_at"] = task.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if task.CompletedAt != nil {
		data["completed_at"] = task.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if task.ExpiresAt != nil {
		data["expires_at"] = task.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if task.CanceledAt != nil {
		data["canceled_at"] = task.CanceledAt.Format("2006-01-02T15:04:05Z07:00")
	}

	c.JSON(200, gin.H{"code": 0, "data": data})
}

func (r *Router) getTaskLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	// 检查任务是否存在
	var task models.ExportTask
	if err := r.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "任务不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	// 查询任务日志
	var logs []models.TaskLog
	if err := r.db.Where("task_id = ?", id).Order("created_at ASC").Find(&logs).Error; err != nil {
		r.logger.Error("failed to get task logs", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 构建响应
	items := make([]gin.H, len(logs))
	for i, log := range logs {
		items[i] = gin.H{
			"level":      log.LogLevel,
			"message":    log.Message,
			"created_at": log.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"task_id": id,
			"logs":    items,
		},
	})
}

func (r *Router) listTiDBConfigs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	var configs []models.TiDBConfig

	// 统计总数
	if err := r.db.Model(&models.TiDBConfig{}).Count(&total).Error; err != nil {
		r.logger.Error("failed to count TiDB configs", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := r.db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&configs).Error; err != nil {
		r.logger.Error("failed to list TiDB configs", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total": total,
			"items": configs,
		},
	})
}

type createTiDBConfigRequest struct {
	TenantID int64  `json:"tenant_id" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Database string `json:"database" binding:"required"`
	SSLMode  string `json:"ssl_mode"`
	Status   int8   `json:"status"`
}

func (r *Router) createTiDBConfig(c *gin.Context) {
	var req createTiDBConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败: "+err.Error())
		return
	}

	// 设置默认值
	if req.Port == 0 {
		req.Port = 4000
	}
	if req.SSLMode == "" {
		req.SSLMode = "disabled"
	}
	if req.Status == 0 {
		req.Status = 1
	}

	// 加密密码
	passwordEncrypted, err := r.encryptor.Encrypt(req.Password)
	if err != nil {
		r.logger.Error("failed to encrypt password", zap.Error(err))
		utils.InternalError(c, "加密密码失败")
		return
	}

	config := &models.TiDBConfig{
		TenantID:          req.TenantID,
		Name:              req.Name,
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		PasswordEncrypted: passwordEncrypted,
		Database:          req.Database,
		SSLMode:           req.SSLMode,
		Status:            req.Status,
	}

	if err := r.db.Create(config).Error; err != nil {
		r.logger.Error("failed to create TiDB config", zap.Error(err))
		utils.InternalError(c, "创建失败")
		return
	}

	c.JSON(201, gin.H{"code": 0, "message": "created", "data": gin.H{"id": config.ID}})
}

type updateTiDBConfigRequest struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
	Status   int8   `json:"status"`
}

func (r *Router) updateTiDBConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var config models.TiDBConfig
	if err := r.db.First(&config, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "配置不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	var req updateTiDBConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Host != "" {
		updates["host"] = req.Host
	}
	if req.Port > 0 {
		updates["port"] = req.Port
	}
	if req.Username != "" {
		updates["username"] = req.Username
	}
	if req.Password != "" {
		passwordEncrypted, err := r.encryptor.Encrypt(req.Password)
		if err != nil {
			r.logger.Error("failed to encrypt password", zap.Error(err))
			utils.InternalError(c, "加密密码失败")
			return
		}
		updates["password_encrypted"] = passwordEncrypted
	}
	if req.Database != "" {
		updates["database"] = req.Database
	}
	if req.SSLMode != "" {
		updates["ssl_mode"] = req.SSLMode
	}
	if req.Status != 0 {
		updates["status"] = req.Status
	}

	if len(updates) > 0 {
		if err := r.db.Model(&config).Updates(updates).Error; err != nil {
			r.logger.Error("failed to update TiDB config", zap.Error(err))
			utils.InternalError(c, "更新失败")
			return
		}
	}

	c.JSON(200, gin.H{"code": 0, "message": "updated"})
}

func (r *Router) deleteTiDBConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	result := r.db.Delete(&models.TiDBConfig{}, id)
	if result.Error != nil {
		r.logger.Error("failed to delete TiDB config", zap.Error(result.Error))
		utils.InternalError(c, "删除失败")
		return
	}
	if result.RowsAffected == 0 {
		utils.NotFound(c, "配置不存在")
		return
	}

	c.JSON(200, gin.H{"code": 0, "message": "deleted"})
}

func (r *Router) listS3Configs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	var configs []models.S3Config

	// 统计总数
	if err := r.db.Model(&models.S3Config{}).Count(&total).Error; err != nil {
		r.logger.Error("failed to count S3 configs", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := r.db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&configs).Error; err != nil {
		r.logger.Error("failed to list S3 configs", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total": total,
			"items": configs,
		},
	})
}

type createS3ConfigRequest struct {
	TenantID    int64  `json:"tenant_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Endpoint    string `json:"endpoint" binding:"required"`
	Bucket      string `json:"bucket" binding:"required"`
	Region      string `json:"region"`
	AccessKeyID string `json:"access_key_id" binding:"required"`
	SecretKey   string `json:"secret_access_key" binding:"required"`
	PathPrefix  string `json:"path_prefix"`
	Status      int8   `json:"status"`
}

func (r *Router) createS3Config(c *gin.Context) {
	var req createS3ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败: "+err.Error())
		return
	}

	// 设置默认值
	if req.Status == 0 {
		req.Status = 1
	}

	// 加密密钥
	secretKeyEncrypted, err := r.encryptor.Encrypt(req.SecretKey)
	if err != nil {
		r.logger.Error("failed to encrypt secret key", zap.Error(err))
		utils.InternalError(c, "加密密钥失败")
		return
	}

	config := &models.S3Config{
		TenantID:           req.TenantID,
		Name:               req.Name,
		Endpoint:           req.Endpoint,
		AccessKey:          req.AccessKeyID,
		SecretKeyEncrypted: secretKeyEncrypted,
		Bucket:             req.Bucket,
		Region:             req.Region,
		PathPrefix:         req.PathPrefix,
		Status:             req.Status,
	}

	if err := r.db.Create(config).Error; err != nil {
		r.logger.Error("failed to create S3 config", zap.Error(err))
		utils.InternalError(c, "创建失败")
		return
	}

	c.JSON(201, gin.H{"code": 0, "message": "created", "data": gin.H{"id": config.ID}})
}

type updateS3ConfigRequest struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Bucket      string `json:"bucket"`
	Region      string `json:"region"`
	AccessKeyID string `json:"access_key_id"`
	SecretKey   string `json:"secret_access_key"`
	PathPrefix  string `json:"path_prefix"`
	Status      int8   `json:"status"`
}

func (r *Router) updateS3Config(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var config models.S3Config
	if err := r.db.First(&config, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "配置不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	var req updateS3ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数验证失败: "+err.Error())
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Endpoint != "" {
		updates["endpoint"] = req.Endpoint
	}
	if req.Bucket != "" {
		updates["bucket"] = req.Bucket
	}
	if req.Region != "" {
		updates["region"] = req.Region
	}
	if req.AccessKeyID != "" {
		updates["access_key"] = req.AccessKeyID
	}
	if req.SecretKey != "" {
		secretKeyEncrypted, err := r.encryptor.Encrypt(req.SecretKey)
		if err != nil {
			r.logger.Error("failed to encrypt secret key", zap.Error(err))
			utils.InternalError(c, "加密密钥失败")
			return
		}
		updates["secret_key_encrypted"] = secretKeyEncrypted
	}
	if req.PathPrefix != "" {
		updates["path_prefix"] = req.PathPrefix
	}
	if req.Status != 0 {
		updates["status"] = req.Status
	}

	if len(updates) > 0 {
		if err := r.db.Model(&config).Updates(updates).Error; err != nil {
			r.logger.Error("failed to update S3 config", zap.Error(err))
			utils.InternalError(c, "更新失败")
			return
		}
	}

	c.JSON(200, gin.H{"code": 0, "message": "updated"})
}

func (r *Router) deleteS3Config(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	result := r.db.Delete(&models.S3Config{}, id)
	if result.Error != nil {
		r.logger.Error("failed to delete S3 config", zap.Error(result.Error))
		utils.InternalError(c, "删除失败")
		return
	}
	if result.RowsAffected == 0 {
		utils.NotFound(c, "配置不存在")
		return
	}

	c.JSON(200, gin.H{"code": 0, "message": "deleted"})
}

func (r *Router) listAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 过滤条件
	action := c.Query("action")
	resourceType := c.Query("resource_type")
	adminID := c.Query("admin_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := r.db.Model(&models.AuditLog{})

	// 应用过滤条件
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if adminID != "" {
		query = query.Where("admin_id = ?", adminID)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate+" 00:00:00")
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate+" 23:59:59")
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		r.logger.Error("failed to count audit logs", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 分页查询
	var logs []models.AuditLog
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		r.logger.Error("failed to list audit logs", zap.Error(err))
		utils.InternalError(c, "查询失败")
		return
	}

	// 构建响应
	items := make([]gin.H, len(logs))
	for i, log := range logs {
		items[i] = gin.H{
			"id":            log.ID,
			"admin_id":      log.AdminID,
			"tenant_id":     log.TenantID,
			"action":        log.Action,
			"resource_type": log.ResourceType,
			"resource_id":   log.ResourceID,
			"request_ip":    log.RequestIP,
			"result":        log.Result,
			"created_at":    log.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if log.ErrorMessage != "" {
			items[i]["error_message"] = log.ErrorMessage
		}
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"items":     items,
		},
	})
}

// recordAuditLog 记录审计日志
func (r *Router) recordAuditLog(c *gin.Context, action, resourceType string, resourceID int64, requestData interface{}, result string) {
	adminID := c.GetInt64(string(middleware.ContextKeyAdminID))

	// 序列化请求数据
	var requestDataStr string
	if requestData != nil {
		if bytes, err := jsonMarshal(requestData); err == nil {
			requestDataStr = string(bytes)
		}
	}

	auditLog := &models.AuditLog{
		AdminID:      adminID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		RequestIP:    c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
		RequestData:  requestDataStr,
		Result:       result,
	}

	if err := r.db.Create(auditLog).Error; err != nil {
		r.logger.Error("failed to create audit log", zap.Error(err))
	}
}

// jsonMarshal 安全的JSON序列化
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
