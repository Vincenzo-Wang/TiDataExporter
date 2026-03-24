package task

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"claw-export-platform/models"
	"claw-export-platform/pkg/queue"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TaskManager 任务管理器
type TaskManager struct {
	db        *gorm.DB
	queue     *queue.Queue
	logger    *zap.Logger

	// 正在执行的任务（用于取消）
	runningTasks   map[int64]*RunningTask
	runningTasksMu sync.RWMutex

	// 超时检查配置
	timeoutCheckInterval time.Duration
	taskTimeout          time.Duration
}

// RunningTask 正在执行的任务信息
type RunningTask struct {
	TaskID      int64
	StartedAt   time.Time
	CancelFunc  context.CancelFunc
	ProcessPID  int // Dumpling进程PID（用于强制终止）
}

// ManagerConfig 任务管理器配置
type ManagerConfig struct {
	DB                   *gorm.DB
	Queue                *queue.Queue
	Logger               *zap.Logger
	TimeoutCheckInterval time.Duration // 超时检查间隔
	TaskTimeout          time.Duration // 任务超时时间
}

// NewTaskManager 创建任务管理器
func NewTaskManager(cfg ManagerConfig) *TaskManager {
	if cfg.TimeoutCheckInterval == 0 {
		cfg.TimeoutCheckInterval = 1 * time.Minute
	}
	if cfg.TaskTimeout == 0 {
		cfg.TaskTimeout = 2 * time.Hour
	}

	return &TaskManager{
		db:                   cfg.DB,
		queue:                cfg.Queue,
		logger:               cfg.Logger,
		runningTasks:         make(map[int64]*RunningTask),
		timeoutCheckInterval: cfg.TimeoutCheckInterval,
		taskTimeout:          cfg.TaskTimeout,
	}
}

// RegisterRunningTask 注册正在执行的任务
func (m *TaskManager) RegisterRunningTask(taskID int64, cancelFunc context.CancelFunc, pid int) {
	m.runningTasksMu.Lock()
	defer m.runningTasksMu.Unlock()

	m.runningTasks[taskID] = &RunningTask{
		TaskID:     taskID,
		StartedAt:  time.Now(),
		CancelFunc: cancelFunc,
		ProcessPID: pid,
	}

	m.logger.Info("task registered as running",
		zap.Int64("task_id", taskID),
		zap.Int("pid", pid),
	)
}

// UnregisterRunningTask 取消注册任务
func (m *TaskManager) UnregisterRunningTask(taskID int64) {
	m.runningTasksMu.Lock()
	defer m.runningTasksMu.Unlock()

	delete(m.runningTasks, taskID)
	m.logger.Info("task unregistered", zap.Int64("task_id", taskID))
}

// CancelTask 取消任务
func (m *TaskManager) CancelTask(ctx context.Context, taskID int64, reason string) error {
	// 获取任务信息
	var task models.ExportTask
	if err := m.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// 检查任务状态
	switch task.Status {
	case models.TaskStatusSuccess, models.TaskStatusFailed, models.TaskStatusCanceled:
		return fmt.Errorf("task already completed with status: %s", task.Status)
	case models.TaskStatusPending:
		// pending状态直接取消
		return m.cancelPendingTask(ctx, &task, reason)
	case models.TaskStatusRunning:
		// running状态需要终止执行
		return m.cancelRunningTask(ctx, &task, reason)
	default:
		return fmt.Errorf("unknown task status: %s", task.Status)
	}
}

// cancelPendingTask 取消待执行任务
func (m *TaskManager) cancelPendingTask(ctx context.Context, task *models.ExportTask, reason string) error {
	now := time.Now()
	if err := m.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"status":        models.TaskStatusCanceled,
		"canceled_at":   now,
		"cancel_reason": reason,
	}).Error; err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	m.logger.Info("pending task canceled",
		zap.Int64("task_id", task.ID),
		zap.String("reason", reason),
	)

	return nil
}

// cancelRunningTask 取消正在执行的任务
func (m *TaskManager) cancelRunningTask(ctx context.Context, task *models.ExportTask, reason string) error {
	// 获取正在执行的任务信息
	m.runningTasksMu.RLock()
	runningTask, exists := m.runningTasks[task.ID]
	m.runningTasksMu.RUnlock()

	if !exists {
		// 任务可能在其他Worker执行，只更新状态
		m.logger.Warn("running task not found in local registry",
			zap.Int64("task_id", task.ID),
		)
		return m.cancelPendingTask(ctx, task, reason)
	}

	// 调用取消函数
	if runningTask.CancelFunc != nil {
		runningTask.CancelFunc()
	}

	// 如果有进程PID，尝试终止进程
	if runningTask.ProcessPID > 0 {
		if err := m.terminateProcess(runningTask.ProcessPID); err != nil {
			m.logger.Warn("failed to terminate process",
				zap.Int64("task_id", task.ID),
				zap.Int("pid", runningTask.ProcessPID),
				zap.Error(err),
			)
		}
	}

	// 更新任务状态
	now := time.Now()
	if err := m.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"status":        models.TaskStatusCanceled,
		"canceled_at":   now,
		"cancel_reason": reason,
	}).Error; err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// 从运行列表中移除
	m.UnregisterRunningTask(task.ID)

	m.logger.Info("running task canceled",
		zap.Int64("task_id", task.ID),
		zap.Int("pid", runningTask.ProcessPID),
		zap.String("reason", reason),
	)

	return nil
}

// terminateProcess 终止进程
func (m *TaskManager) terminateProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}

	// 查找进程
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// 首先尝试优雅终止 (SIGTERM)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		m.logger.Warn("failed to send SIGTERM, trying SIGKILL",
			zap.Int("pid", pid),
			zap.Error(err),
		)
		// 如果 SIGTERM 失败，尝试强制终止
		if err := process.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	}

	m.logger.Info("process terminated", zap.Int("pid", pid))
	return nil
}

// StartTimeoutChecker 启动超时检查器
func (m *TaskManager) StartTimeoutChecker(ctx context.Context) {
	ticker := time.NewTicker(m.timeoutCheckInterval)
	defer ticker.Stop()

	m.logger.Info("timeout checker started",
		zap.Duration("interval", m.timeoutCheckInterval),
		zap.Duration("timeout", m.taskTimeout),
	)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("timeout checker stopped")
			return
		case <-ticker.C:
			m.checkTimeoutTasks(ctx)
		}
	}
}

// checkTimeoutTasks 检查超时任务
func (m *TaskManager) checkTimeoutTasks(ctx context.Context) {
	timeoutThreshold := time.Now().Add(-m.taskTimeout)

	// 查询超时的运行中任务
	var tasks []models.ExportTask
	if err := m.db.WithContext(ctx).
		Where("status = ?", models.TaskStatusRunning).
		Where("started_at < ?", timeoutThreshold).
		Find(&tasks).Error; err != nil {
		m.logger.Error("failed to query timeout tasks", zap.Error(err))
		return
	}

	if len(tasks) == 0 {
		return
	}

	m.logger.Warn("found timeout tasks", zap.Int("count", len(tasks)))

	for _, task := range tasks {
		if err := m.handleTimeoutTask(ctx, &task); err != nil {
			m.logger.Error("failed to handle timeout task",
				zap.Int64("task_id", task.ID),
				zap.Error(err),
			)
		}
	}
}

// handleTimeoutTask 处理超时任务
func (m *TaskManager) handleTimeoutTask(ctx context.Context, task *models.ExportTask) error {
	m.logger.Warn("handling timeout task",
		zap.Int64("task_id", task.ID),
		zap.Time("started_at", *task.StartedAt),
	)

	// 尝试取消任务
	if err := m.CancelTask(ctx, task.ID, "任务执行超时"); err != nil {
		// 如果取消失败，直接更新状态
		now := time.Now()
		if err := m.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
			"status":        models.TaskStatusFailed,
			"error_message": "任务执行超时",
			"completed_at":  now,
		}).Error; err != nil {
			return fmt.Errorf("failed to update timeout task: %w", err)
		}
	}

	// 记录日志
	logEntry := &models.TaskLog{
		TaskID:   task.ID,
		LogLevel: "ERROR",
		Message:  "任务执行超时，已自动取消",
	}
	m.db.WithContext(ctx).Create(logEntry)

	return nil
}

// RetryTask 重试任务
func (m *TaskManager) RetryTask(ctx context.Context, taskID int64) error {
	var task models.ExportTask
	if err := m.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// 检查任务状态
	if task.Status != models.TaskStatusFailed {
		return fmt.Errorf("only failed tasks can be retried, current status: %s", task.Status)
	}

	// 检查重试次数
	if task.RetryCount >= task.MaxRetries {
		return fmt.Errorf("max retries (%d) exceeded", task.MaxRetries)
	}

	// 增加重试计数并重置状态
	task.RetryCount++
	task.Status = models.TaskStatusPending
	task.ErrorMessage = ""
	task.StartedAt = nil
	task.CompletedAt = nil

	if err := m.db.WithContext(ctx).Save(&task).Error; err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// 重新入队
	if err := m.queue.Enqueue(ctx, &task); err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	m.logger.Info("task scheduled for retry",
		zap.Int64("task_id", task.ID),
		zap.Int("retry_count", task.RetryCount),
	)

	return nil
}

// GetTaskStatus 获取任务状态
func (m *TaskManager) GetTaskStatus(ctx context.Context, taskID int64) (*models.ExportTask, error) {
	var task models.ExportTask
	if err := m.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}
	return &task, nil
}

// ListTasksByStatus 按状态列出任务
func (m *TaskManager) ListTasksByStatus(ctx context.Context, tenantID int64, status string, page, pageSize int) ([]models.ExportTask, int64, error) {
	var tasks []models.ExportTask
	var total int64

	query := m.db.WithContext(ctx).Model(&models.ExportTask{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}
