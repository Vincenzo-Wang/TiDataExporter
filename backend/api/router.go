package api

import (
	"strconv"

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

// 以下是管理API的简化实现

func (r *Router) listTenants(c *gin.Context) {
	// TODO: 实现租户列表
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"total": 0, "items": []interface{}{}}})
}

func (r *Router) createTenant(c *gin.Context) {
	// TODO: 实现创建租户
	c.JSON(201, gin.H{"code": 0, "message": "created"})
}

func (r *Router) getTenant(c *gin.Context) {
	// TODO: 实现获取租户
	c.JSON(200, gin.H{"code": 0, "data": gin.H{}})
}

func (r *Router) updateTenant(c *gin.Context) {
	// TODO: 实现更新租户
	c.JSON(200, gin.H{"code": 0, "message": "updated"})
}

func (r *Router) deleteTenant(c *gin.Context) {
	// TODO: 实现删除租户
	c.JSON(200, gin.H{"code": 0, "message": "deleted"})
}

func (r *Router) listTasks(c *gin.Context) {
	// TODO: 实现任务列表
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"total": 0, "items": []interface{}{}}})
}

func (r *Router) getTask(c *gin.Context) {
	// TODO: 实现获取任务
	c.JSON(200, gin.H{"code": 0, "data": gin.H{}})
}

func (r *Router) getTaskLogs(c *gin.Context) {
	// TODO: 实现获取任务日志
	c.JSON(200, gin.H{"code": 0, "data": []interface{}{}})
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
	// TODO: 实现审计日志列表
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"total": 0, "items": []interface{}{}}})
}
