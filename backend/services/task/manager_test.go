package task

import (
	"context"
	"testing"
	"time"

	"claw-export-platform/models"
	"claw-export-platform/pkg/queue"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	err = db.AutoMigrate(&models.ExportTask{}, &models.TaskLog{}, &models.Tenant{}, &models.TiDBConfig{}, &models.S3Config{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func setupTestQueue(t *testing.T) *queue.Queue {
	// 创建Redis客户端（仅用于测试，实际测试中可能需要mock）
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	logger := zap.NewNop()

	q := queue.NewQueue(nil, logger) // 使用nil避免实际Redis连接

	// 注意：实际测试应该使用mock或嵌入式Redis
	_ = rdb // 避免未使用警告
	return q
}

func TestTaskManager_CancelTask_Pending(t *testing.T) {
	db := setupTestDB(t)

	// 创建测试任务
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusPending,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	// 创建任务管理器
	manager := NewTaskManager(ManagerConfig{
		DB:     db,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 取消任务
	err := manager.CancelTask(context.Background(), task.ID, "用户取消")
	if err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}

	// 验证状态
	var updatedTask models.ExportTask
	db.First(&updatedTask, task.ID)

	if updatedTask.Status != models.TaskStatusCanceled {
		t.Errorf("expected status %s, got %s", models.TaskStatusCanceled, updatedTask.Status)
	}

	if updatedTask.CancelReason != "用户取消" {
		t.Errorf("expected cancel_reason '用户取消', got '%s'", updatedTask.CancelReason)
	}

	if updatedTask.CanceledAt == nil {
		t.Error("canceled_at should not be nil")
	}
}

func TestTaskManager_CancelTask_AlreadyCompleted(t *testing.T) {
	db := setupTestDB(t)

	// 创建已完成的任务
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusSuccess,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	manager := NewTaskManager(ManagerConfig{
		DB:     db,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 尝试取消已完成的任务
	err := manager.CancelTask(context.Background(), task.ID, "用户取消")
	if err == nil {
		t.Error("expected error when canceling completed task")
	}
}

func TestTaskManager_CancelTask_NotFound(t *testing.T) {
	db := setupTestDB(t)

	manager := NewTaskManager(ManagerConfig{
		DB:     db,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 尝试取消不存在的任务
	err := manager.CancelTask(context.Background(), 99999, "用户取消")
	if err == nil {
		t.Error("expected error when canceling non-existent task")
	}
}

func TestTaskManager_RegisterRunningTask(t *testing.T) {
	manager := NewTaskManager(ManagerConfig{
		DB:     nil,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 注册任务
	manager.RegisterRunningTask(1, nil, 12345)

	// 验证任务已注册
	manager.runningTasksMu.RLock()
	_, exists := manager.runningTasks[1]
	manager.runningTasksMu.RUnlock()

	if !exists {
		t.Error("task should be registered")
	}

	// 取消注册
	manager.UnregisterRunningTask(1)

	manager.runningTasksMu.RLock()
	_, exists = manager.runningTasks[1]
	manager.runningTasksMu.RUnlock()

	if exists {
		t.Error("task should be unregistered")
	}
}

func TestTaskManager_RetryTask(t *testing.T) {
	db := setupTestDB(t)

	// 创建失败的任务
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusFailed,
		ErrorMessage:   "执行失败",
		RetryCount:     0,
		MaxRetries:     3,
		RetentionHours: 168,
	}
	db.Create(task)

	manager := NewTaskManager(ManagerConfig{
		DB:     db,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 重试任务
	err := manager.RetryTask(context.Background(), task.ID)
	if err != nil {
		// 由于没有队列，可能会失败，这是预期的
		t.Logf("RetryTask returned expected error: %v", err)
	}

	// 验证重试计数增加
	var updatedTask models.ExportTask
	db.First(&updatedTask, task.ID)

	if updatedTask.RetryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", updatedTask.RetryCount)
	}

	if updatedTask.Status != models.TaskStatusPending {
		t.Errorf("expected status pending, got %s", updatedTask.Status)
	}
}

func TestTaskManager_RetryTask_MaxRetriesExceeded(t *testing.T) {
	db := setupTestDB(t)

	// 创建已达最大重试次数的任务
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusFailed,
		ErrorMessage:   "执行失败",
		RetryCount:     3,
		MaxRetries:     3,
		RetentionHours: 168,
	}
	db.Create(task)

	manager := NewTaskManager(ManagerConfig{
		DB:     db,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 尝试重试
	err := manager.RetryTask(context.Background(), task.ID)
	if err == nil {
		t.Error("expected error when max retries exceeded")
	}
}

func TestTaskManager_GetTaskStatus(t *testing.T) {
	db := setupTestDB(t)

	// 创建任务
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusPending,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	manager := NewTaskManager(ManagerConfig{
		DB:     db,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 获取任务状态
	result, err := manager.GetTaskStatus(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTaskStatus failed: %v", err)
	}

	if result.ID != task.ID {
		t.Errorf("expected task ID %d, got %d", task.ID, result.ID)
	}
}

func TestTaskManager_ListTasksByStatus(t *testing.T) {
	db := setupTestDB(t)

	// 创建多个任务
	for i := 0; i < 5; i++ {
		task := &models.ExportTask{
			TenantID:       1,
			TiDBConfigID:   1,
			S3ConfigID:     1,
			SqlText:        "SELECT 1",
			Status:         models.TaskStatusPending,
			RetentionHours: 168,
			MaxRetries:     3,
		}
		db.Create(task)
	}

	for i := 0; i < 3; i++ {
		task := &models.ExportTask{
			TenantID:       1,
			TiDBConfigID:   1,
			S3ConfigID:     1,
			SqlText:        "SELECT 1",
			Status:         models.TaskStatusSuccess,
			RetentionHours: 168,
			MaxRetries:     3,
		}
		db.Create(task)
	}

	manager := NewTaskManager(ManagerConfig{
		DB:     db,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	// 列出pending任务
	tasks, total, err := manager.ListTasksByStatus(context.Background(), 1, models.TaskStatusPending, 1, 10)
	if err != nil {
		t.Fatalf("ListTasksByStatus failed: %v", err)
	}

	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}

	if len(tasks) != 5 {
		t.Errorf("expected 5 tasks, got %d", len(tasks))
	}

	// 列出success任务
	tasks, total, err = manager.ListTasksByStatus(context.Background(), 1, models.TaskStatusSuccess, 1, 10)
	if err != nil {
		t.Fatalf("ListTasksByStatus failed: %v", err)
	}

	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
}

func TestTaskManager_TimeoutCheck(t *testing.T) {
	db := setupTestDB(t)

	// 创建超时的运行中任务
	oldTime := time.Now().Add(-3 * time.Hour)
	task := &models.ExportTask{
		TenantID:       1,
		TiDBConfigID:   1,
		S3ConfigID:     1,
		SqlText:        "SELECT 1",
		Status:         models.TaskStatusRunning,
		StartedAt:      &oldTime,
		RetentionHours: 168,
		MaxRetries:     3,
	}
	db.Create(task)

	manager := NewTaskManager(ManagerConfig{
		DB:          db,
		Queue:       nil,
		Logger:      zap.NewNop(),
		TaskTimeout: 2 * time.Hour,
	})

	// 执行超时检查
	manager.checkTimeoutTasks(context.Background())

	// 验证任务状态已更新
	var updatedTask models.ExportTask
	db.First(&updatedTask, task.ID)

	if updatedTask.Status != models.TaskStatusCanceled && updatedTask.Status != models.TaskStatusFailed {
		t.Errorf("expected status canceled or failed, got %s", updatedTask.Status)
	}
}

func TestNewTaskManager_DefaultConfig(t *testing.T) {
	manager := NewTaskManager(ManagerConfig{
		DB:     nil,
		Queue:  nil,
		Logger: zap.NewNop(),
	})

	if manager.timeoutCheckInterval != 1*time.Minute {
		t.Errorf("expected default timeout check interval 1m, got %v", manager.timeoutCheckInterval)
	}

	if manager.taskTimeout != 2*time.Hour {
		t.Errorf("expected default task timeout 2h, got %v", manager.taskTimeout)
	}
}
