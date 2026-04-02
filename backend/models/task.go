package models

import (
	"time"

	"gorm.io/gorm"
)

// 任务状态常量
const (
	TaskStatusPending  = "pending"
	TaskStatusRunning  = "running"
	TaskStatusSuccess  = "success"
	TaskStatusFailed   = "failed"
	TaskStatusCanceled = "canceled"
	TaskStatusExpired  = "expired"
)

// ExportTask 导出任务
type ExportTask struct {
	ID              int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID        int64      `gorm:"not null;index:idx_tenant_status,column:tenant_id" json:"tenant_id"`
	TaskName        string     `gorm:"type:varchar(255);column:task_name" json:"task_name"`
	TiDBConfigID    int64      `gorm:"not null;column:tidb_config_id" json:"tidb_config_id"`
	S3ConfigID      int64      `gorm:"not null;column:s3_config_id" json:"s3_config_id"`
	SqlText         string     `gorm:"type:text;not null;column:sql_text" json:"sql_text"`
	Filetype        string     `gorm:"type:varchar(10);default:sql" json:"filetype"`
	Compress        string     `gorm:"type:varchar(20)" json:"compress"`
	RetentionHours  int        `gorm:"default:168;column:retention_hours" json:"retention_hours"`
	Priority        int        `gorm:"default:5;index:idx_priority" json:"priority"`
	Status          string     `gorm:"type:varchar(20);default:pending;index:idx_tenant_status" json:"status"`
	FileURL         string     `gorm:"type:varchar(500);column:file_url" json:"file_url"`
	FileURLs        string     `gorm:"type:json;column:file_urls" json:"file_urls"`
	FileSize        int64      `gorm:"column:file_size" json:"file_size"`
	RowCount        int64      `gorm:"column:row_count" json:"row_count"`
	ErrorMessage    string     `gorm:"type:text;column:error_message" json:"error_message"`
	CancelReason    string     `gorm:"type:text;column:cancel_reason" json:"cancel_reason"`
	RetryCount      int        `gorm:"default:0;column:retry_count" json:"retry_count"`
	MaxRetries      int        `gorm:"default:3;column:max_retries" json:"max_retries"`
	StartedAt       *time.Time `gorm:"precision:0" json:"started_at"`
	CompletedAt     *time.Time `gorm:"precision:0" json:"completed_at"`
	ExpiresAt       *time.Time `gorm:"precision:0;index:idx_expires_at" json:"expires_at"`
	CanceledAt      *time.Time `gorm:"precision:0" json:"canceled_at"`
	CreatedAt       time.Time  `gorm:"precision:0;index:idx_created_at" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"precision:0;autoUpdateTime" json:"updated_at"`

	// 关联字段（不存储在数据库）
	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

func (ExportTask) TableName() string {
	return "export_tasks"
}

func (t *ExportTask) BeforeCreate(tx *gorm.DB) error {
	if t.FileURLs == "" {
		t.FileURLs = "[]"
	}
	return nil
}

// TaskLog 任务执行日志
type TaskLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID    int64     `gorm:"not null;index:idx_task_created,column:task_id" json:"task_id"`
	LogLevel  string    `gorm:"type:varchar(10);column:log_level" json:"log_level"`
	Message   string    `gorm:"type:text" json:"message"`
	CreatedAt time.Time `gorm:"precision:0;index:idx_task_created" json:"created_at"`
}

func (TaskLog) TableName() string {
	return "task_logs"
}

// AuditLog 审计日志
type AuditLog struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID     int64     `gorm:"index:idx_tenant_created,column:tenant_id" json:"tenant_id"`
	AdminID      int64     `gorm:"index:idx_admin_created,column:admin_id" json:"admin_id"`
	Action       string    `gorm:"type:varchar(50);not null" json:"action"`
	ResourceType string    `gorm:"type:varchar(50);index:idx_resource,column:resource_type" json:"resource_type"`
	ResourceID   int64     `gorm:"index:idx_resource,column:resource_id" json:"resource_id"`
	RequestIP    string    `gorm:"type:varchar(45);column:request_ip" json:"request_ip"`
	UserAgent    string    `gorm:"type:varchar(255);column:user_agent" json:"user_agent"`
	RequestData  string    `gorm:"type:text;column:request_data" json:"request_data"`
	Result       string    `gorm:"type:varchar(20);column:result" json:"result"`
	ErrorMessage string    `gorm:"type:text;column:error_message" json:"error_message"`
	CreatedAt    time.Time `gorm:"precision:0;index:idx_tenant_created;index:idx_admin_created" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
