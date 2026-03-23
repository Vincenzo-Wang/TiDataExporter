package cleanup

import (
	"context"
	"testing"
	"time"

	"claw-export-platform/models"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCleanupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	err = db.AutoMigrate(&models.ExportTask{}, &models.TaskLog{}, &models.S3Config{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestNewCleaner_DefaultConfig(t *testing.T) {
	cleaner := NewCleaner(CleanerConfig{
		DB:     nil,
		Logger: zap.NewNop(),
	})

	if cleaner.checkInterval != 1*time.Hour {
		t.Errorf("expected default check interval 1h, got %v", cleaner.checkInterval)
	}

	if cleaner.logRetention != 30*24*time.Hour {
		t.Errorf("expected default log retention 720h, got %v", cleaner.logRetention)
	}
}

func TestCleaner_CleanExpiredFiles(t *testing.T) {
	db := setupCleanupTestDB(t)

	// 创建过期任务
	pastTime := time.Now().Add(-24 * time.Hour)
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusSuccess,
		ExpiresAt:      &pastTime,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	cleaner := NewCleaner(CleanerConfig{
		DB:        db,
		S3Clients: make(map[int64]interface{ DeleteByPrefix(ctx context.Context, prefix string) error }),
		Logger:    zap.NewNop(),
	})

	// 执行清理
	count, err := cleaner.cleanExpiredFiles(context.Background())
	if err != nil {
		t.Fatalf("cleanExpiredFiles failed: %v", err)
	}

	// 由于没有S3客户端，文件不会被真正删除
	// 但任务状态应该更新
	if count != 1 {
		t.Errorf("expected 1 cleaned task, got %d", count)
	}

	// 验证任务状态
	var updatedTask models.ExportTask
	db.First(&updatedTask, task.ID)

	if updatedTask.Status != models.TaskStatusExpired {
		t.Errorf("expected status expired, got %s", updatedTask.Status)
	}
}

func TestCleaner_CleanOldTaskLogs(t *testing.T) {
	db := setupCleanupTestDB(t)

	// 创建旧日志
	oldTime := time.Now().Add(-35 * 24 * time.Hour)
	log := &models.TaskLog{
		TaskID:   1,
		LogLevel: "INFO",
		Message:  "old log entry",
	}
	db.Create(&log)

	// 手动设置created_at
	db.Model(log).Update("created_at", oldTime)

	cleaner := NewCleaner(CleanerConfig{
		DB:           db,
		Logger:       zap.NewNop(),
		LogRetention: 30 * 24 * time.Hour,
	})

	// 执行清理
	count, err := cleaner.cleanOldTaskLogs(context.Background())
	if err != nil {
		t.Fatalf("cleanOldTaskLogs failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 cleaned log, got %d", count)
	}

	// 验证日志已删除
	var logCount int64
	db.Model(&models.TaskLog{}).Count(&logCount)
	if logCount != 0 {
		t.Errorf("expected 0 logs remaining, got %d", logCount)
	}
}

func TestCleaner_GetExpiringTasks(t *testing.T) {
	db := setupCleanupTestDB(t)

	// 创建即将过期的任务
	expiresAt := time.Now().Add(12 * time.Hour)
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusSuccess,
		ExpiresAt:      &expiresAt,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	// 创建不会过期的任务
	farFuture := time.Now().Add(30 * 24 * time.Hour)
	task2 := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 2",
		Status:         models.TaskStatusSuccess,
		ExpiresAt:      &farFuture,
		RetentionHours: 720,
		MaxRetries:     3,
	}
	db.Create(task2)

	cleaner := NewCleaner(CleanerConfig{
		DB:     db,
		Logger: zap.NewNop(),
	})

	// 获取24小时内过期的任务
	tasks, err := cleaner.GetExpiringTasks(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("GetExpiringTasks failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 expiring task, got %d", len(tasks))
	}

	if len(tasks) > 0 && tasks[0].ID != task.ID {
		t.Errorf("expected task ID %d, got %d", task.ID, tasks[0].ID)
	}
}

func TestCleaner_ExtendTaskExpiration(t *testing.T) {
	db := setupCleanupTestDB(t)

	// 创建任务
	expiresAt := time.Now().Add(24 * time.Hour)
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusSuccess,
		ExpiresAt:      &expiresAt,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	cleaner := NewCleaner(CleanerConfig{
		DB:     db,
		Logger: zap.NewNop(),
	})

	// 延长过期时间
	err := cleaner.ExtendTaskExpiration(context.Background(), task.ID, 24)
	if err != nil {
		t.Fatalf("ExtendTaskExpiration failed: %v", err)
	}

	// 验证过期时间已延长
	var updatedTask models.ExportTask
	db.First(&updatedTask, task.ID)

	expectedExpiry := expiresAt.Add(24 * time.Hour)
	diff := updatedTask.ExpiresAt.Sub(expectedExpiry)
	if diff.Abs() > time.Minute {
		t.Errorf("expected expires_at around %v, got %v", expectedExpiry, updatedTask.ExpiresAt)
	}
}

func TestCleaner_ExtendTaskExpiration_NonSuccessfulTask(t *testing.T) {
	db := setupCleanupTestDB(t)

	// 创建失败任务
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusFailed,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	cleaner := NewCleaner(CleanerConfig{
		DB:     db,
		Logger: zap.NewNop(),
	})

	// 尝试延长失败任务的过期时间
	err := cleaner.ExtendTaskExpiration(context.Background(), task.ID, 24)
	if err == nil {
		t.Error("expected error when extending expiration for non-successful task")
	}
}
