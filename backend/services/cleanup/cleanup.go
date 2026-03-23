package cleanup

import (
	"context"
	"fmt"
	"time"

	"claw-export-platform/models"
	"claw-export-platform/services/s3"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Cleaner 文件清理器
type Cleaner struct {
	db        *gorm.DB
	s3Clients map[int64]*s3.Client // 按租户ID缓存的S3客户端
	logger    *zap.Logger

	// 清理配置
	checkInterval time.Duration // 检查间隔
	logRetention  time.Duration // 日志保留时间
}

// CleanerConfig 清理器配置
type CleanerConfig struct {
	DB            *gorm.DB
	S3Clients     map[int64]*s3.Client
	Logger        *zap.Logger
	CheckInterval time.Duration // 默认1小时
	LogRetention  time.Duration // 默认30天
}

// NewCleaner 创建清理器
func NewCleaner(cfg CleanerConfig) *Cleaner {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 1 * time.Hour
	}
	if cfg.LogRetention == 0 {
		cfg.LogRetention = 30 * 24 * time.Hour // 30天
	}

	if cfg.S3Clients == nil {
		cfg.S3Clients = make(map[int64]*s3.Client)
	}

	return &Cleaner{
		db:            cfg.DB,
		s3Clients:     cfg.S3Clients,
		logger:        cfg.Logger,
		checkInterval: cfg.CheckInterval,
		logRetention:  cfg.LogRetention,
	}
}

// Start 启动清理器（阻塞运行）
func (c *Cleaner) Start(ctx context.Context) {
	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	c.logger.Info("file cleaner started",
		zap.Duration("interval", c.checkInterval),
		zap.Duration("log_retention", c.logRetention),
	)

	// 立即执行一次
	c.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("file cleaner stopped")
			return
		case <-ticker.C:
			c.runCleanup(ctx)
		}
	}
}

// runCleanup 执行清理
func (c *Cleaner) runCleanup(ctx context.Context) {
	c.logger.Info("starting cleanup cycle")

	// 1. 清理过期文件
	if count, err := c.cleanExpiredFiles(ctx); err != nil {
		c.logger.Error("failed to clean expired files", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("cleaned expired files", zap.Int("count", count))
	}

	// 2. 清理过期任务记录（可选）
	if count, err := c.cleanOldTaskLogs(ctx); err != nil {
		c.logger.Error("failed to clean old task logs", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("cleaned old task logs", zap.Int("count", count))
	}

	// 3. 清理孤立的临时文件（如果有）
	if count, err := c.cleanOrphanedFiles(ctx); err != nil {
		c.logger.Error("failed to clean orphaned files", zap.Error(err))
	} else if count > 0 {
		c.logger.Info("cleaned orphaned files", zap.Int("count", count))
	}

	c.logger.Info("cleanup cycle completed")
}

// cleanExpiredFiles 清理过期文件
func (c *Cleaner) cleanExpiredFiles(ctx context.Context) (int, error) {
	// 查询过期任务
	var tasks []models.ExportTask
	if err := c.db.WithContext(ctx).
		Where("status = ?", models.TaskStatusSuccess).
		Where("expires_at < ?", time.Now()).
		Find(&tasks).Error; err != nil {
		return 0, fmt.Errorf("failed to query expired tasks: %w", err)
	}

	if len(tasks) == 0 {
		return 0, nil
	}

	c.logger.Info("found expired tasks to clean", zap.Int("count", len(tasks)))

	cleanedCount := 0
	for _, task := range tasks {
		if err := c.cleanTaskFiles(ctx, &task); err != nil {
			c.logger.Error("failed to clean task files",
				zap.Int64("task_id", task.ID),
				zap.Error(err),
			)
			continue
		}

		// 更新任务状态为已过期
		if err := c.db.WithContext(ctx).Model(&task).Update("status", models.TaskStatusExpired).Error; err != nil {
			c.logger.Error("failed to update task status to expired",
				zap.Int64("task_id", task.ID),
				zap.Error(err),
			)
			continue
		}

		cleanedCount++
	}

	return cleanedCount, nil
}

// cleanTaskFiles 清理单个任务的文件
func (c *Cleaner) cleanTaskFiles(ctx context.Context, task *models.ExportTask) error {
	// 如果没有文件URL，跳过
	if task.FileURL == "" {
		return nil
	}

	// 获取S3配置
	var s3Config models.S3Config
	if err := c.db.WithContext(ctx).First(&s3Config, task.S3ConfigID).Error; err != nil {
		return fmt.Errorf("failed to get s3 config: %w", err)
	}

	// 获取S3客户端
	s3Client, exists := c.s3Clients[s3Config.ID]
	if !exists {
		// 创建新的S3客户端
		// 注意：这里需要解密SecretKey，简化实现中跳过
		c.logger.Warn("s3 client not found in cache, skipping file deletion",
			zap.Int64("config_id", s3Config.ID),
		)
		return nil
	}

	// 删除S3上的文件
	prefix := fmt.Sprintf("exports/%d/", task.ID)
	if err := s3Client.DeleteByPrefix(ctx, prefix); err != nil {
		return fmt.Errorf("failed to delete s3 files: %w", err)
	}

	c.logger.Info("deleted task files from s3",
		zap.Int64("task_id", task.ID),
		zap.String("prefix", prefix),
	)

	return nil
}

// cleanOldTaskLogs 清理旧任务日志
func (c *Cleaner) cleanOldTaskLogs(ctx context.Context) (int, error) {
	threshold := time.Now().Add(-c.logRetention)

	result := c.db.WithContext(ctx).
		Where("created_at < ?", threshold).
		Delete(&models.TaskLog{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete old task logs: %w", result.Error)
	}

	return int(result.RowsAffected), nil
}

// cleanOrphanedFiles 清理孤立文件（数据库中没有记录但S3上存在的文件）
func (c *Cleaner) cleanOrphanedFiles(ctx context.Context) (int, error) {
	// 这是一个可选的高级功能
	// 需要列出S3上的文件并与数据库记录比较
	// 简化实现中跳过
	return 0, nil
}

// CleanTaskNow 立即清理指定任务的文件
func (c *Cleaner) CleanTaskNow(ctx context.Context, taskID int64) error {
	var task models.ExportTask
	if err := c.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	return c.cleanTaskFiles(ctx, &task)
}

// GetExpiringTasks 获取即将过期的任务
func (c *Cleaner) GetExpiringTasks(ctx context.Context, within time.Duration) ([]models.ExportTask, error) {
	threshold := time.Now().Add(within)

	var tasks []models.ExportTask
	if err := c.db.WithContext(ctx).
		Where("status = ?", models.TaskStatusSuccess).
		Where("expires_at BETWEEN ? AND ?", time.Now(), threshold).
		Find(&tasks).Error; err != nil {
		return nil, err
	}

	return tasks, nil
}

// ExtendTaskExpiration 延长任务过期时间
func (c *Cleaner) ExtendTaskExpiration(ctx context.Context, taskID int64, additionalHours int) error {
	var task models.ExportTask
	if err := c.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	if task.Status != models.TaskStatusSuccess {
		return fmt.Errorf("can only extend expiration for successful tasks")
	}

	if task.ExpiresAt == nil {
		return fmt.Errorf("task has no expiration time")
	}

	newExpiresAt := task.ExpiresAt.Add(time.Duration(additionalHours) * time.Hour)
	if err := c.db.WithContext(ctx).Model(&task).Update("expires_at", newExpiresAt).Error; err != nil {
		return fmt.Errorf("failed to update expiration: %w", err)
	}

	c.logger.Info("extended task expiration",
		zap.Int64("task_id", taskID),
		zap.Time("new_expires_at", newExpiresAt),
	)

	return nil
}
