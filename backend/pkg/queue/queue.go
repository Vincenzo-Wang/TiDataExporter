package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"claw-export-platform/models"
	"claw-export-platform/pkg/redis"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TaskMessage 任务消息
type TaskMessage struct {
	TaskID       int64  `json:"task_id"`
	TenantID     int64  `json:"tenant_id"`
	SqlText      string `json:"sql_text"`
	Filetype     string `json:"filetype"`
	Compress     string `json:"compress"`
	Priority     int    `json:"priority"`
	TiDBConfigID int64  `json:"tidb_config_id"`
	S3ConfigID   int64  `json:"s3_config_id"`
}

// Queue 任务队列
type Queue struct {
	redis  *redis.Client
	logger *zap.Logger
}

// NewQueue 创建任务队列
func NewQueue(redisClient *redis.Client, logger *zap.Logger) *Queue {
	return &Queue{
		redis:  redisClient,
		logger: logger,
	}
}

// Enqueue 将任务加入队列
func (q *Queue) Enqueue(ctx context.Context, task *models.ExportTask) error {
	msg := TaskMessage{
		TaskID:       task.ID,
		TenantID:     task.TenantID,
		SqlText:      task.SqlText,
		Filetype:     task.Filetype,
		Compress:     task.Compress,
		Priority:     task.Priority,
		TiDBConfigID: task.TiDBConfigID,
		S3ConfigID:   task.S3ConfigID,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal task message: %w", err)
	}

	fields := map[string]interface{}{
		"task_id":    task.ID,
		"data":       string(data),
		"priority":   task.Priority,
		"created_at": time.Now().Unix(),
	}

	msgID, err := q.redis.AddMessage(ctx, fields)
	if err != nil {
		return fmt.Errorf("failed to add message to stream: %w", err)
	}

	q.logger.Info("task enqueued",
		zap.Int64("task_id", task.ID),
		zap.String("msg_id", msgID),
		zap.Int("priority", task.Priority),
	)

	return nil
}

// Dequeue 从队列获取任务
func (q *Queue) Dequeue(ctx context.Context) (*TaskMessage, string, error) {
	messages, err := q.redis.ReadMessages(ctx, 1)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read messages: %w", err)
	}

	if len(messages) == 0 {
		return nil, "", nil
	}

	msg := messages[0]
	data, ok := msg.Fields["data"].(string)
	if !ok {
		return nil, "", fmt.Errorf("invalid message format: missing data field")
	}

	var taskMsg TaskMessage
	if err := json.Unmarshal([]byte(data), &taskMsg); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal task message: %w", err)
	}

	q.logger.Info("task dequeued",
		zap.Int64("task_id", taskMsg.TaskID),
		zap.String("msg_id", msg.ID),
	)

	return &taskMsg, msg.ID, nil
}

// Ack 确认任务完成
func (q *Queue) Ack(ctx context.Context, msgID string) error {
	if err := q.redis.AckMessage(ctx, msgID); err != nil {
		return fmt.Errorf("failed to ack message: %w", err)
	}
	q.logger.Info("message acknowledged", zap.String("msg_id", msgID))
	return nil
}

// Nack 拒绝任务（重新入队）
func (q *Queue) Nack(ctx context.Context, msgID string) error {
	// Redis Stream没有直接的Nack，消息会留在pending列表
	// 可以通过Claim重新获取
	q.logger.Warn("message nacked", zap.String("msg_id", msgID))
	return nil
}

// GetPendingTasks 获取超时待处理任务
func (q *Queue) GetPendingTasks(ctx context.Context) ([]goredis.XPendingExt, error) {
	return q.redis.GetPendingMessages(ctx)
}

// ClaimPendingTask 认领超时任务
func (q *Queue) ClaimPendingTask(ctx context.Context, msgID string) (*TaskMessage, string, error) {
	msg, err := q.redis.ClaimMessage(ctx, msgID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to claim message: %w", err)
	}
	if msg == nil {
		return nil, "", nil
	}

	data, ok := msg.Values["data"].(string)
	if !ok {
		return nil, "", fmt.Errorf("invalid message format: missing data field")
	}

	var taskMsg TaskMessage
	if err := json.Unmarshal([]byte(data), &taskMsg); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal task message: %w", err)
	}

	return &taskMsg, msg.ID, nil
}

// Initialize 初始化队列（确保Stream和Consumer Group存在）
func (q *Queue) Initialize(ctx context.Context) error {
	return q.redis.EnsureStreamAndGroup(ctx)
}

// PendingTimeout 返回待处理消息的认领超时阈值
func (q *Queue) PendingTimeout() time.Duration {
	return q.redis.PendingTimeout()
}
