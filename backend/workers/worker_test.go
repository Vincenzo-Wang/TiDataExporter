package workers

import (
	"testing"

	"claw-export-platform/models"
)

func TestTaskStatusConstants(t *testing.T) {
	// 验证任务状态常量
	statuses := map[string]string{
		models.TaskStatusPending:   "pending",
		models.TaskStatusRunning:   "running",
		models.TaskStatusSuccess:   "success",
		models.TaskStatusFailed:    "failed",
		models.TaskStatusCanceled:  "canceled",
		models.TaskStatusExpired:   "expired",
	}

	for status, expected := range statuses {
		if status != expected {
			t.Errorf("expected status %q, got %q", expected, status)
		}
	}
}
