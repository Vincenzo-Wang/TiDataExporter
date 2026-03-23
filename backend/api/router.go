package api

import (
	"claw-export-platform/api/middleware"
	v1 "claw-export-platform/api/v1"
	"claw-export-platform/config"
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
	// TODO: 实现TiDB配置列表
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"total": 0, "items": []interface{}{}}})
}

func (r *Router) createTiDBConfig(c *gin.Context) {
	// TODO: 实现创建TiDB配置
	c.JSON(201, gin.H{"code": 0, "message": "created"})
}

func (r *Router) updateTiDBConfig(c *gin.Context) {
	// TODO: 实现更新TiDB配置
	c.JSON(200, gin.H{"code": 0, "message": "updated"})
}

func (r *Router) deleteTiDBConfig(c *gin.Context) {
	// TODO: 实现删除TiDB配置
	c.JSON(200, gin.H{"code": 0, "message": "deleted"})
}

func (r *Router) listS3Configs(c *gin.Context) {
	// TODO: 实现S3配置列表
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"total": 0, "items": []interface{}{}}})
}

func (r *Router) createS3Config(c *gin.Context) {
	// TODO: 实现创建S3配置
	c.JSON(201, gin.H{"code": 0, "message": "created"})
}

func (r *Router) updateS3Config(c *gin.Context) {
	// TODO: 实现更新S3配置
	c.JSON(200, gin.H{"code": 0, "message": "updated"})
}

func (r *Router) deleteS3Config(c *gin.Context) {
	// TODO: 实现删除S3配置
	c.JSON(200, gin.H{"code": 0, "message": "deleted"})
}

func (r *Router) listAuditLogs(c *gin.Context) {
	// TODO: 实现审计日志列表
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"total": 0, "items": []interface{}{}}})
}
