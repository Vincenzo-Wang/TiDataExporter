package workers

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"claw-export-platform/models"
	"claw-export-platform/pkg/encryption"
	"claw-export-platform/pkg/queue"
	"claw-export-platform/services/export"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Worker 任务执行Worker
type Worker struct {
	id        int
	db        *gorm.DB
	queue     *queue.Queue
	encryptor *encryption.Encryptor
	logger    *zap.Logger

	// 工作目录
	workDir string

	// 控制信号
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// Config Worker配置
type Config struct {
	ID        int
	DB        *gorm.DB
	Queue     *queue.Queue
	Encryptor *encryption.Encryptor
	WorkDir   string
	Logger    *zap.Logger
}

// NewWorker 创建Worker
func NewWorker(cfg Config) *Worker {
	return &Worker{
		id:        cfg.ID,
		db:        cfg.DB,
		queue:     cfg.Queue,
		encryptor: cfg.Encryptor,
		workDir:   cfg.WorkDir,
		logger:    cfg.Logger,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动Worker
func (w *Worker) Start() {
	w.wg.Add(1)
	go w.run()
	w.logger.Info("worker started", zap.Int("worker_id", w.id))
}

// Stop 停止Worker
func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	w.logger.Info("worker stopped", zap.Int("worker_id", w.id))
}

func (w *Worker) run() {
	defer w.wg.Done()

	// 待处理任务检查定时器
	pendingCheckTicker := time.NewTicker(5 * time.Minute)
	defer pendingCheckTicker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-pendingCheckTicker.C:
			w.checkPendingMessages()
		default:
			w.processTask()
		}
	}
}

func (w *Worker) processTask() {
	ctx := context.Background()

	// 从队列获取任务
	taskMsg, msgID, err := w.queue.Dequeue(ctx)
	if err != nil {
		w.logger.Error("failed to dequeue task", zap.Error(err))
		time.Sleep(1 * time.Second)
		return
	}

	if taskMsg == nil {
		// 无任务，短暂休眠
		time.Sleep(100 * time.Millisecond)
		return
	}

	// 处理任务
	if err := w.handleTask(ctx, taskMsg); err != nil {
		w.logger.Error("task execution failed",
			zap.Int64("task_id", taskMsg.TaskID),
			zap.Error(err),
		)
		w.handleTaskFailure(ctx, taskMsg, err)
	}

	// 确认消息
	if err := w.queue.Ack(ctx, msgID); err != nil {
		w.logger.Error("failed to ack message",
			zap.String("msg_id", msgID),
			zap.Error(err),
		)
	}
}

func (w *Worker) handleTask(ctx context.Context, msg *queue.TaskMessage) error {
	// 获取任务详情
	var task models.ExportTask
	if err := w.db.WithContext(ctx).First(&task, msg.TaskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// 检查任务状态
	if task.Status != models.TaskStatusPending {
		w.logger.Info("task already processed, skipping",
			zap.Int64("task_id", task.ID),
			zap.String("status", task.Status),
		)
		return nil
	}

	// 更新任务状态为running
	now := time.Now()
	if err := w.db.WithContext(ctx).Model(&task).Updates(map[string]interface{}{
		"status":     models.TaskStatusRunning,
		"started_at": now,
	}).Error; err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// 获取TiDB配置
	var tidbConfig models.TiDBConfig
	if err := w.db.WithContext(ctx).First(&tidbConfig, msg.TiDBConfigID).Error; err != nil {
		return fmt.Errorf("tidb config not found: %w", err)
	}

	// 获取S3配置
	var s3Config models.S3Config
	if err := w.db.WithContext(ctx).First(&s3Config, msg.S3ConfigID).Error; err != nil {
		return fmt.Errorf("s3 config not found: %w", err)
	}

	// 创建执行器并执行
	executor := export.NewExecutor(w.db, w.encryptor, w.workDir, w.logger)
	result, err := executor.Execute(ctx, task.ID, &tidbConfig, &s3Config, msg.SqlText, msg.Filetype, msg.Compress)
	if err != nil {
		return err
	}

	// 更新任务成功状态
	completedAt := time.Now()
	expiresAt := completedAt.Add(time.Duration(task.RetentionHours) * time.Hour)

	fileURLsJSON, err := json.Marshal(result.Files)
	if err != nil {
		return fmt.Errorf("failed to marshal task files: %w", err)
	}

	if err := w.db.WithContext(ctx).Model(&task).Updates(map[string]interface{}{
		"status":       models.TaskStatusSuccess,
		"file_url":     result.FileURL,
		"file_urls":    string(fileURLsJSON),
		"file_size":    result.FileSize,
		"completed_at": completedAt,
		"expires_at":   expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("failed to update task success: %w", err)
	}

	w.logger.Info("task completed successfully",
		zap.Int64("task_id", task.ID),
		zap.Int64("file_size", result.FileSize),
		zap.Duration("duration", result.Duration),
	)

	return nil
}

func (w *Worker) handleTaskFailure(ctx context.Context, msg *queue.TaskMessage, execErr error) {
	var task models.ExportTask
	if err := w.db.WithContext(ctx).First(&task, msg.TaskID).Error; err != nil {
		return
	}

	// 检查是否可以重试
	if task.RetryCount < task.MaxRetries {
		// 增加重试计数
		task.RetryCount++
		task.Status = models.TaskStatusPending

		if err := w.db.WithContext(ctx).Save(&task).Error; err != nil {
			w.logger.Error("failed to update task for retry", zap.Error(err))
			return
		}

		// 重新入队（延迟重试）
		delay := time.Duration(1<<task.RetryCount) * time.Second
		time.AfterFunc(delay, func() {
			if err := w.queue.Enqueue(context.Background(), &task); err != nil {
				w.logger.Error("failed to re-enqueue task", zap.Error(err))
			}
		})

		w.logger.Info("task scheduled for retry",
			zap.Int64("task_id", task.ID),
			zap.Int("retry_count", task.RetryCount),
			zap.Duration("delay", delay),
		)
		return
	}

	// 更新任务失败状态
	if err := w.db.WithContext(ctx).Model(&task).Updates(map[string]interface{}{
		"status":        models.TaskStatusFailed,
		"error_message": execErr.Error(),
		"completed_at":  time.Now(),
	}).Error; err != nil {
		w.logger.Error("failed to update task failure", zap.Error(err))
	}
}

func (w *Worker) checkPendingMessages() {
	ctx := context.Background()

	pending, err := w.queue.GetPendingTasks(ctx)
	if err != nil {
		w.logger.Error("failed to get pending tasks", zap.Error(err))
		return
	}

	for _, p := range pending {
		// 检查是否超时
		if p.Idle < w.queue.PendingTimeout() {
			continue
		}

		w.logger.Warn("found pending task, claiming",
			zap.String("msg_id", p.ID),
			zap.Duration("idle", p.Idle),
		)

		// 认领超时消息
		taskMsg, msgID, err := w.queue.ClaimPendingTask(ctx, p.ID)
		if err != nil {
			w.logger.Error("failed to claim pending task", zap.Error(err))
			continue
		}

		if taskMsg != nil {
			if err := w.handleTask(ctx, taskMsg); err != nil {
				w.logger.Error("failed to handle claimed task", zap.Error(err))
			}
			if err := w.queue.Ack(ctx, msgID); err != nil {
				w.logger.Error("failed to ack claimed task", zap.Error(err))
			}
		}
	}
}

// WorkerPool Worker池
type WorkerPool struct {
	workers []*Worker
	logger  *zap.Logger
}

// NewWorkerPool 创建Worker池
func NewWorkerPool(count int, db *gorm.DB, q *queue.Queue, encryptor *encryption.Encryptor, workDir string, logger *zap.Logger) *WorkerPool {
	pool := &WorkerPool{
		workers: make([]*Worker, count),
		logger:  logger,
	}

	for i := 0; i < count; i++ {
		pool.workers[i] = NewWorker(Config{
			ID:        i + 1,
			DB:        db,
			Queue:     q,
			Encryptor: encryptor,
			WorkDir:   workDir,
			Logger:    logger.Named(fmt.Sprintf("worker-%d", i+1)),
		})
	}

	return pool
}

// Start 启动所有Worker
func (p *WorkerPool) Start() {
	for _, w := range p.workers {
		w.Start()
	}
	p.logger.Info("worker pool started", zap.Int("count", len(p.workers)))
}

// Stop 停止所有Worker
func (p *WorkerPool) Stop() {
	for _, w := range p.workers {
		w.Stop()
	}
	p.logger.Info("worker pool stopped")
}

// Run 运行Worker池（阻塞，直到收到终止信号）
func (p *WorkerPool) Run() {
	p.Start()

	// 等待终止信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	p.logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	p.Stop()
}
