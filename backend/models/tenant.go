package models

import (
	"time"

	"gorm.io/gorm"
)

// Tenant 租户
type Tenant struct {
	ID                int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Name              string         `gorm:"type:varchar(100);not null" json:"name"`
	APIKey            string         `gorm:"type:varchar(64);uniqueIndex;not null;column:api_key" json:"-"`
	APISecretEncrypted string        `gorm:"type:text;not null;column:api_secret_encrypted" json:"-"`
	Status            int8           `gorm:"type:tinyint;default:1;index" json:"status"`
	CreatedAt         time.Time      `gorm:"precision:0" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"precision:0;autoUpdateTime" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Tenant) TableName() string {
	return "tenants"
}

// TenantQuota 租户配额
type TenantQuota struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID            int64     `gorm:"uniqueIndex;not null;column:tenant_id" json:"tenant_id"`
	MaxConcurrentTasks  int       `gorm:"default:5;column:max_concurrent_tasks" json:"max_concurrent_tasks"`
	MaxDailyTasks       int       `gorm:"default:100;column:max_daily_tasks" json:"max_daily_tasks"`
	MaxDailySizeGB      float64   `gorm:"type:decimal(10,2);default:50;column:max_daily_size_gb" json:"max_daily_size_gb"`
	MaxRetentionHours   int       `gorm:"default:720;column:max_retention_hours" json:"max_retention_hours"`
	CreatedAt           time.Time `gorm:"precision:0" json:"created_at"`
	UpdatedAt           time.Time `gorm:"precision:0;autoUpdateTime" json:"updated_at"`
}

func (TenantQuota) TableName() string {
	return "tenant_quotas"
}
