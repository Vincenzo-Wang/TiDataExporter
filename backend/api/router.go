package api

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
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
		adminAuthGroup.POST("/tenants/:id/regenerate-keys", r.regenerateTenantKeys)

		// 任务管理
		adminAuthGroup.GET("/tasks", r.listTasks)
		adminAuthGroup.GET("/tasks/:id", r.getTask)
		adminAuthGroup.GET("/tasks/:id/logs", r.getTaskLogs)
		adminAuthGroup.POST("/tasks/:id/cancel", r.cancelTask)
		adminAuthGroup.POST("/tasks/:id/retry", r.retryTask)

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

		// 统计信息
		adminAuthGroup.GET("/statistics/overview", r.getStatisticsOverview)
		adminAuthGroup.GET("/statistics/daily", r.getDailyStatistics)
		adminAuthGroup.GET("/statistics/tenants", r.getTenantStatistics)
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

	// 获取所有租户ID
	tenantIDs := make([]int64, len(tenants))
	for i, t := range tenants {
		tenantIDs[i] = t.ID
	}

	// 批量查询配额
	var quotas []models.TenantQuota
	quotaMap := make(map[int64]models.TenantQuota)
	if err := r.db.Where("tenant_id IN ?", tenantIDs).Find(&quotas).Error; err != nil {
		r.logger.Error("failed to fetch quotas", zap.Error(err))
	}
	for _, q := range quotas {
		quotaMap[q.TenantID] = q
	}

	// 统计每日和每月任务数
	type TaskCount struct {
		TenantID int64
		Count    int64
	}
	var dailyCounts []TaskCount
	var monthlyCounts []TaskCount

	// 今日任务数
	today := time.Now().Format("2006-01-02")
	r.db.Model(&models.ExportTask{}).
		Select("tenant_id, COUNT(*) as count").
		Where("tenant_id IN ? AND DATE(created_at) = ?", tenantIDs, today).
		Group("tenant_id").
		Scan(&dailyCounts)

	// 本月任务数
	monthStart := time.Now().Format("2006-01") + "-01"
	r.db.Model(&models.ExportTask{}).
		Select("tenant_id, COUNT(*) as count").
		Where("tenant_id IN ? AND DATE(created_at) >= ?", tenantIDs, monthStart).
		Group("tenant_id").
		Scan(&monthlyCounts)

	dailyCountMap := make(map[int64]int64)
	for _, dc := range dailyCounts {
		dailyCountMap[dc.TenantID] = dc.Count
	}
	monthlyCountMap := make(map[int64]int64)
	for _, mc := range monthlyCounts {
		monthlyCountMap[mc.TenantID] = mc.Count
	}

	// 构建响应
	items := make([]gin.H, len(tenants))
	for i, t := range tenants {
		quota := quotaMap[t.ID]
		items[i] = gin.H{
			"id":               t.ID,
			"name":             t.Name,
			"code":             t.Code,
			"contact_email":    t.ContactEmail,
			"api_key":          t.APIKey,
			"status":           t.Status,
			"quota_daily":      quota.MaxDailyTasks,
			"quota_monthly":    quota.MaxDailyTasks * 30, // 月配额按日配额*30计算
			"quota_used_today": dailyCountMap[t.ID],
			"quota_used_month": monthlyCountMap[t.ID],
			"max_concurrent":   quota.MaxConcurrentTasks,
			"max_size_gb":      quota.MaxDailySizeGB,
			"retention_hours":  quota.MaxRetentionHours,
			"created_at":       t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
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
	Name         string `json:"name" binding:"required"`
	Code         string `json:"code" binding:"required"`
	ContactEmail string `json:"contact_email"`
	QuotaDaily   int    `json:"quota_daily"`
	QuotaMonthly int    `json:"quota_monthly"`
	Status       int8   `json:"status"`
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

	// 设置默认值
	if req.Status == 0 {
		req.Status = 1
	}
	if req.QuotaDaily == 0 {
		req.QuotaDaily = 100
	}
	if req.QuotaMonthly == 0 {
		req.QuotaMonthly = 3000
	}

	// 创建租户
	tenant := &models.Tenant{
		Name:               req.Name,
		Code:               req.Code,
		ContactEmail:       req.ContactEmail,
		APIKey:             apiKey,
		APISecretEncrypted: apiSecretEncrypted,
		Status:             req.Status,
	}

	if err := r.db.Create(tenant).Error; err != nil {
		r.logger.Error("failed to create tenant", zap.Error(err))
		utils.InternalError(c, "创建租户失败")
		return
	}

	// 创建配额
	quota := &models.TenantQuota{
		TenantID:           tenant.ID,
		MaxConcurrentTasks: 5,
		MaxDailyTasks:      req.QuotaDaily,
		MaxDailySizeGB:     50,
		MaxRetentionHours:  720,
	}
	if err := r.db.Create(quota).Error; err != nil {
		r.logger.Error("failed to create tenant quota", zap.Error(err))
		// 不回滚租户创建，只记录错误
	}

	// 记录审计日志
	r.recordAuditLog(c, "create", "tenant", tenant.ID, gin.H{"name": req.Name, "code": req.Code}, "success")

	c.JSON(201, gin.H{
		"code":    0,
		"message": "租户创建成功",
		"data": gin.H{
			"id":            tenant.ID,
			"tenant_id":     tenant.ID,
			"api_key":       apiKey,
			"api_secret":    apiSecret, // 只在创建时返回一次
			"name":          tenant.Name,
			"code":          tenant.Code,
			"contact_email": tenant.ContactEmail,
			"status":        tenant.Status,
			"quota_daily":   req.QuotaDaily,
			"quota_monthly": req.QuotaMonthly,
			"created_at":    tenant.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
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

	// 获取配置名称映射
	tidbConfigIDs := make([]int64, 0)
	s3ConfigIDs := make([]int64, 0)
	for _, task := range tasks {
		tidbConfigIDs = append(tidbConfigIDs, task.TiDBConfigID)
		s3ConfigIDs = append(s3ConfigIDs, task.S3ConfigID)
	}

	tidbConfigMap := make(map[int64]string)
	s3ConfigMap := make(map[int64]string)

	if len(tidbConfigIDs) > 0 {
		var tidbConfigs []models.TiDBConfig
		r.db.Select("id, name").Where("id IN ?", tidbConfigIDs).Find(&tidbConfigs)
		for _, cfg := range tidbConfigs {
			tidbConfigMap[cfg.ID] = cfg.Name
		}
	}

	if len(s3ConfigIDs) > 0 {
		var s3Configs []models.S3Config
		r.db.Select("id, name").Where("id IN ?", s3ConfigIDs).Find(&s3Configs)
		for _, cfg := range s3Configs {
			s3ConfigMap[cfg.ID] = cfg.Name
		}
	}

	// 计算进度
	calculateProgress := func(task models.ExportTask) int {
		switch task.Status {
		case models.TaskStatusPending:
			return 0
		case models.TaskStatusRunning:
			return 50
		case models.TaskStatusSuccess:
			return 100
		case models.TaskStatusFailed, models.TaskStatusCanceled, models.TaskStatusExpired:
			return 100
		default:
			return 0
		}
	}

	// 构建响应
	items := make([]gin.H, len(tasks))
	for i, task := range tasks {
		files := buildAdminTaskFiles(task)
		item := gin.H{
			"task_id":         task.ID,
			"task_name":       task.TaskName,
			"tenant_id":       task.TenantID,
			"tidb_config_id":  task.TiDBConfigID,
			"s3_config_id":    task.S3ConfigID,
			"filetype":        task.Filetype,
			"compress":        task.Compress,
			"retention_hours": task.RetentionHours,
			"priority":        task.Priority,
			"status":          task.Status,
			"progress":        calculateProgress(task),
			"file_url":        task.FileURL,
			"file_count":      len(files),
			"file_size":       task.FileSize,
			"row_count":       task.RowCount,
			"retry_count":     task.RetryCount,
			"max_retries":     task.MaxRetries,
			"error_message":   task.ErrorMessage,
			"created_at":      task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"updated_at":      task.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		// 添加租户名称
		if task.Tenant.ID != 0 {
			item["tenant_name"] = task.Tenant.Name
		}

		// 添加配置名称
		if name, ok := tidbConfigMap[task.TiDBConfigID]; ok {
			item["tidb_config_name"] = name
		}
		if name, ok := s3ConfigMap[task.S3ConfigID]; ok {
			item["s3_config_name"] = name
		}

		if task.StartedAt != nil {
			item["started_at"] = task.StartedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if task.CompletedAt != nil {
			item["completed_at"] = task.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if task.ExpiresAt != nil {
			item["expires_at"] = task.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
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

	// 计算进度
	calculateProgress := func(task models.ExportTask) int {
		switch task.Status {
		case models.TaskStatusPending:
			return 0
		case models.TaskStatusRunning:
			return 50
		case models.TaskStatusSuccess:
			return 100
		case models.TaskStatusFailed, models.TaskStatusCanceled, models.TaskStatusExpired:
			return 100
		default:
			return 0
		}
	}

	files := buildAdminTaskFiles(task)
	data := gin.H{
		"task_id":          task.ID,
		"task_name":        task.TaskName,
		"tenant_id":        task.TenantID,
		"tenant_name":      task.Tenant.Name,
		"tidb_config_id":   task.TiDBConfigID,
		"tidb_config_name": tidbConfig.Name,
		"s3_config_id":     task.S3ConfigID,
		"s3_config_name":   s3Config.Name,
		"sql_text":         task.SqlText,
		"filetype":         task.Filetype,
		"compress":         task.Compress,
		"retention_hours":  task.RetentionHours,
		"priority":         task.Priority,
		"status":           task.Status,
		"progress":         calculateProgress(task),
		"file_url":         task.FileURL,
		"files":            files,
		"file_count":       len(files),
		"file_size":        task.FileSize,
		"row_count":        task.RowCount,
		"retry_count":      task.RetryCount,
		"max_retries":      task.MaxRetries,
		"error_message":    task.ErrorMessage,
		"cancel_reason":    task.CancelReason,
		"created_at":       task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":       task.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
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

	// 构建响应（排除敏感字段）
	items := make([]gin.H, len(configs))
	for i, cfg := range configs {
		items[i] = gin.H{
			"id":         cfg.ID,
			"tenant_id":  cfg.TenantID,
			"name":       cfg.Name,
			"host":       cfg.Host,
			"port":       cfg.Port,
			"username":   cfg.Username,
			"database":   cfg.Database,
			"ssl_mode":   cfg.SSLMode,
			"ssl_ca":     cfg.SSLCA,
			"ssl_cert":   cfg.SSLCert,
			"ssl_key":    cfg.SSLKey,
			"status":     cfg.Status,
			"is_default": cfg.IsDefault,
			"created_at": cfg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"updated_at": cfg.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total": total,
			"items": items,
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
	SSLCA    string `json:"ssl_ca"`
	SSLCert  string `json:"ssl_cert"`
	SSLKey   string `json:"ssl_key"`
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
		SSLCA:             req.SSLCA,
		SSLCert:           req.SSLCert,
		SSLKey:            req.SSLKey,
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
	SSLCA    string `json:"ssl_ca"`
	SSLCert  string `json:"ssl_cert"`
	SSLKey   string `json:"ssl_key"`
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
	if req.SSLCA != "" {
		updates["ssl_ca"] = req.SSLCA
	}
	if req.SSLCert != "" {
		updates["ssl_cert"] = req.SSLCert
	}
	if req.SSLKey != "" {
		updates["ssl_key"] = req.SSLKey
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

	// 构建响应（排除敏感字段）
	items := make([]gin.H, len(configs))
	for i, cfg := range configs {
		items[i] = gin.H{
			"id":          cfg.ID,
			"tenant_id":   cfg.TenantID,
			"name":        cfg.Name,
			"provider":    cfg.Provider,
			"endpoint":    cfg.Endpoint,
			"bucket":      cfg.Bucket,
			"region":      cfg.Region,
			"path_prefix": cfg.PathPrefix,
			"status":      cfg.Status,
			"is_default":  cfg.IsDefault,
			"created_at":  cfg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"updated_at":  cfg.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total": total,
			"items": items,
		},
	})
}

type createS3ConfigRequest struct {
	TenantID    int64  `json:"tenant_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Provider    string `json:"provider"`
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
	if req.Provider == "" {
		req.Provider = "aws"
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
		Provider:           models.ProviderType(req.Provider),
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
	Provider    string `json:"provider"`
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
	if req.Provider != "" {
		updates["provider"] = req.Provider
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
	// PathPrefix 允许设置为空（用于清空前缀）
	updates["path_prefix"] = req.PathPrefix
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

// cancelTask 取消任务
func (r *Router) cancelTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var task models.ExportTask
	if err := r.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "任务不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	// 检查任务状态
	if task.Status != models.TaskStatusPending && task.Status != models.TaskStatusRunning {
		utils.BadRequest(c, "只能取消待处理或运行中的任务")
		return
	}

	// 更新任务状态
	now := time.Now()
	task.Status = models.TaskStatusCanceled
	task.CanceledAt = &now
	task.CancelReason = "管理员手动取消"

	if err := r.db.Save(&task).Error; err != nil {
		r.logger.Error("failed to cancel task", zap.Error(err))
		utils.InternalError(c, "取消失败")
		return
	}

	// 记录审计日志
	r.recordAuditLog(c, "cancel", "task", task.ID, gin.H{"task_id": id}, "success")

	c.JSON(200, gin.H{"code": 0, "message": "任务已取消"})
}

// retryTask 重试任务
func (r *Router) retryTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var task models.ExportTask
	if err := r.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.NotFound(c, "任务不存在")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}

	// 检查任务状态
	if task.Status != models.TaskStatusFailed {
		utils.BadRequest(c, "只能重试失败的任务")
		return
	}

	// 检查重试次数
	if task.RetryCount >= task.MaxRetries {
		utils.BadRequest(c, "已达到最大重试次数")
		return
	}

	// 更新任务状态
	task.Status = models.TaskStatusPending
	task.RetryCount++
	task.ErrorMessage = ""
	task.StartedAt = nil
	task.CompletedAt = nil

	if err := r.db.Save(&task).Error; err != nil {
		r.logger.Error("failed to retry task", zap.Error(err))
		utils.InternalError(c, "重试失败")
		return
	}

	// 记录审计日志
	r.recordAuditLog(c, "retry", "task", task.ID, gin.H{"task_id": id, "retry_count": task.RetryCount}, "success")

	c.JSON(200, gin.H{"code": 0, "message": "任务已重新提交"})
}

// regenerateTenantKeys 重新生成租户密钥
func (r *Router) regenerateTenantKeys(c *gin.Context) {
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

	// 生成新的 API Key 和 API Secret
	newAPIKey := generateAPIKey()
	newAPISecret := generateAPISecret()

	// 加密新的 API Secret
	apiSecretEncrypted, err := r.encryptor.Encrypt(newAPISecret)
	if err != nil {
		r.logger.Error("failed to encrypt api secret", zap.Error(err))
		utils.InternalError(c, "加密密钥失败")
		return
	}

	// 更新租户
	tenant.APIKey = newAPIKey
	tenant.APISecretEncrypted = apiSecretEncrypted

	if err := r.db.Save(&tenant).Error; err != nil {
		r.logger.Error("failed to update tenant keys", zap.Error(err))
		utils.InternalError(c, "更新密钥失败")
		return
	}

	// 记录审计日志
	r.recordAuditLog(c, "regenerate_keys", "tenant", tenant.ID, gin.H{"tenant_id": id}, "success")

	c.JSON(200, gin.H{
		"code":    0,
		"message": "密钥已重新生成",
		"data": gin.H{
			"api_key":    newAPIKey,
			"api_secret": newAPISecret, // 只在创建时返回一次
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

type adminTaskFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func parseAdminTaskFiles(task models.ExportTask) []adminTaskFile {
	if strings.TrimSpace(task.FileURLs) != "" {
		var files []adminTaskFile
		if err := json.Unmarshal([]byte(task.FileURLs), &files); err == nil && len(files) > 0 {
			return files
		}
	}
	if strings.TrimSpace(task.FileURL) == "" {
		return nil
	}
	return []adminTaskFile{{
		Path: task.FileURL,
		Name: filepath.Base(task.FileURL),
		Size: task.FileSize,
	}}
}

func buildAdminTaskFiles(task models.ExportTask) []gin.H {
	taskFiles := parseAdminTaskFiles(task)
	if len(taskFiles) == 0 {
		return nil
	}
	respFiles := make([]gin.H, 0, len(taskFiles))
	for i, file := range taskFiles {
		name := file.Name
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(file.Path)
		}
		respFiles = append(respFiles, gin.H{
			"index": i,
			"name":  name,
			"path":  file.Path,
			"url":   file.Path,
			"size":  file.Size,
		})
	}
	return respFiles
}

// getStatisticsOverview 获取统计概览
func (r *Router) getStatisticsOverview(c *gin.Context) {
	startTime, endTime, err := parseStatisticsRange(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	query := r.buildStatisticsBaseQuery(c, startTime, endTime)

	// 统计各状态任务数量
	type StatusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []StatusCount
	query.Select("status, COUNT(*) as count").
		Group("status").
		Scan(&statusCounts)

	statusMap := make(map[string]int64)
	for _, sc := range statusCounts {
		statusMap[sc.Status] = sc.Count
	}

	var totalTasks int64
	query.Count(&totalTasks)

	type SumResult struct {
		TotalRows int64
		TotalSize int64
	}
	var sumResult SumResult
	r.buildStatisticsBaseQuery(c, startTime, endTime).
		Select("COALESCE(SUM(row_count), 0) as total_rows, COALESCE(SUM(file_size), 0) as total_size").
		Where("status = ?", models.TaskStatusSuccess).
		Scan(&sumResult)

	type AvgDuration struct {
		AvgSeconds float64
	}
	var avgDuration AvgDuration
	r.buildStatisticsBaseQuery(c, startTime, endTime).
		Select("COALESCE(AVG(TIMESTAMPDIFF(SECOND, started_at, completed_at)), 0) as avg_seconds").
		Where("status = ? AND started_at IS NOT NULL AND completed_at IS NOT NULL", models.TaskStatusSuccess).
		Scan(&avgDuration)

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"total_tasks":    totalTasks,
			"pending_tasks":  statusMap[models.TaskStatusPending],
			"running_tasks":  statusMap[models.TaskStatusRunning],
			"success_tasks":  statusMap[models.TaskStatusSuccess],
			"failed_tasks":   statusMap[models.TaskStatusFailed],
			"canceled_tasks": statusMap[models.TaskStatusCanceled],
			"total_rows":     sumResult.TotalRows,
			"total_size":     sumResult.TotalSize,
			"avg_duration":   int64(avgDuration.AvgSeconds),
		},
	})
}

// getDailyStatistics 获取每日统计数据
func (r *Router) getDailyStatistics(c *gin.Context) {
	startTime, endTime, err := parseStatisticsRange(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	type DailyStat struct {
		Date         string `json:"date"`
		TaskCount    int64  `json:"task_count"`
		SuccessCount int64  `json:"success_count"`
		FailedCount  int64  `json:"failed_count"`
		TotalRows    int64  `json:"total_rows"`
		TotalSize    int64  `json:"total_size"`
	}
	var rows []DailyStat

	r.buildStatisticsBaseQuery(c, startTime, endTime).
		Select(`
			DATE(created_at) as date,
			COUNT(*) as task_count,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_count,
			COALESCE(SUM(row_count), 0) as total_rows,
			COALESCE(SUM(file_size), 0) as total_size
		`).
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&rows)

	rowMap := make(map[string]DailyStat, len(rows))
	for _, item := range rows {
		rowMap[item.Date] = item
	}

	dailyStats := make([]DailyStat, 0)
	for d := startTime; !d.After(endTime); d = d.AddDate(0, 0, 1) {
		dateKey := d.Format("2006-01-02")
		if item, ok := rowMap[dateKey]; ok {
			dailyStats = append(dailyStats, item)
			continue
		}
		dailyStats = append(dailyStats, DailyStat{Date: dateKey})
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": dailyStats,
	})
}

// getTenantStatistics 获取租户维度统计
func (r *Router) getTenantStatistics(c *gin.Context) {
	startTime, endTime, err := parseStatisticsRange(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	sortExpr, limit, err := parseTenantStatisticsOptions(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	type TenantStat struct {
		TenantID     int64   `json:"tenant_id"`
		TenantName   string  `json:"tenant_name"`
		TaskCount    int64   `json:"task_count"`
		SuccessCount int64   `json:"success_count"`
		FailedCount  int64   `json:"failed_count"`
		TotalSize    int64   `json:"total_size"`
		SuccessRate  float64 `json:"success_rate"`
		FailureRate  float64 `json:"failure_rate"`
	}

	query := r.db.Table("export_tasks t").
		Where("t.created_at >= ? AND t.created_at < ?", startTime, endTime.AddDate(0, 0, 1))

	tenantID := c.Query("tenant_id")
	if tenantID != "" {
		query = query.Where("t.tenant_id = ?", tenantID)
	}

	var tenantStats []TenantStat
	query = query.Joins("LEFT JOIN tenants tn ON tn.id = t.tenant_id").
		Select(`
			t.tenant_id as tenant_id,
			COALESCE(tn.name, CONCAT('租户-', t.tenant_id)) as tenant_name,
			COUNT(*) as task_count,
			SUM(CASE WHEN t.status = 'success' THEN 1 ELSE 0 END) as success_count,
			SUM(CASE WHEN t.status = 'failed' THEN 1 ELSE 0 END) as failed_count,
			COALESCE(SUM(t.file_size), 0) as total_size,
			CASE WHEN COUNT(*) = 0 THEN 0 ELSE ROUND(SUM(CASE WHEN t.status = 'success' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) END as success_rate,
			CASE WHEN COUNT(*) = 0 THEN 0 ELSE ROUND(SUM(CASE WHEN t.status = 'failed' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 2) END as failure_rate
		`).
		Group("t.tenant_id, tn.name").
		Order(sortExpr)
	if limit > 0 {
		query = query.Limit(limit)
	}
	query.Scan(&tenantStats)

	if tenantStats == nil {
		tenantStats = []TenantStat{}
	}

	c.JSON(200, gin.H{
		"code": 0,
		"data": tenantStats,
	})
}

func (r *Router) buildStatisticsBaseQuery(c *gin.Context, startTime, endTime time.Time) *gorm.DB {
	query := r.db.Model(&models.ExportTask{}).
		Where("created_at >= ? AND created_at < ?", startTime, endTime.AddDate(0, 0, 1))

	tenantID := c.Query("tenant_id")
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}

	return query
}

func parseTenantStatisticsOptions(c *gin.Context) (string, int, error) {
	sortBy := c.DefaultQuery("sort_by", "task_count")
	order := c.DefaultQuery("order", "desc")

	sortFieldMap := map[string]string{
		"task_count":   "task_count",
		"success_count": "success_count",
		"failed_count": "failed_count",
		"success_rate": "success_rate",
		"failure_rate": "failure_rate",
		"total_size":   "total_size",
	}
	field, ok := sortFieldMap[sortBy]
	if !ok {
		return "", 0, fmt.Errorf("sort_by 参数不支持")
	}

	orderUpper := "DESC"
	if order == "asc" {
		orderUpper = "ASC"
	} else if order != "desc" {
		return "", 0, fmt.Errorf("order 参数仅支持 asc/desc")
	}

	limit := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			return "", 0, fmt.Errorf("limit 参数必须是正整数")
		}
		if parsed > 200 {
			parsed = 200
		}
		limit = parsed
	}

	return field + " " + orderUpper, limit, nil
}

func parseStatisticsRange(c *gin.Context) (time.Time, time.Time, error) {
	const dateLayout = "2006-01-02"

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -30).Format(dateLayout)
	}
	if endDate == "" {
		endDate = time.Now().Format(dateLayout)
	}

	startTime, err := time.ParseInLocation(dateLayout, startDate, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start_date 格式错误，应为 YYYY-MM-DD")
	}
	endTime, err := time.ParseInLocation(dateLayout, endDate, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end_date 格式错误，应为 YYYY-MM-DD")
	}
	if endTime.Before(startTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("end_date 不能早于 start_date")
	}

	return startTime, endTime, nil
}
