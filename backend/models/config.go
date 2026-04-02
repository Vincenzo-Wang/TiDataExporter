package models

import (
	"time"

	"gorm.io/gorm"
)

// TiDBConfig TiDB 连接配置
type TiDBConfig struct {
	ID                int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID          int64          `gorm:"not null;uniqueIndex:uk_tenant_name;index:idx_tenant_default;column:tenant_id" json:"tenant_id"`
	Name              string         `gorm:"type:varchar(100);not null;uniqueIndex:uk_tenant_name" json:"name"`
	Host              string         `gorm:"type:varchar(255);not null" json:"host"`
	Port              int            `gorm:"default:4000" json:"port"`
	Username          string         `gorm:"type:varchar(100);not null" json:"username"`
	PasswordEncrypted string         `gorm:"type:text;not null;column:password_encrypted" json:"-"`
	Database          string         `gorm:"type:varchar(100)" json:"database"`
	SSLMode           string         `gorm:"type:varchar(20);default:'disabled';column:ssl_mode" json:"ssl_mode"`
	SSLCA             string         `gorm:"type:varchar(255);column:ssl_ca" json:"ssl_ca"`
	SSLCert           string         `gorm:"type:varchar(255);column:ssl_cert" json:"ssl_cert"`
	SSLKey            string         `gorm:"type:varchar(255);column:ssl_key" json:"ssl_key"`
	Status            int8           `gorm:"type:tinyint;default:1" json:"status"`
	IsDefault         int8           `gorm:"type:tinyint;default:0;index:idx_tenant_default;column:is_default" json:"is_default"`
	CreatedAt         time.Time      `gorm:"precision:0" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"precision:0;autoUpdateTime" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

func (TiDBConfig) TableName() string {
	return "tidb_configs"
}

// ProviderType 存储厂商类型
type ProviderType string

const (
	ProviderAWS    ProviderType = "aws"
	ProviderAliyun ProviderType = "aliyun"
)

// S3Config S3 存储配置
type S3Config struct {
	ID                 int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID           int64          `gorm:"not null;uniqueIndex:uk_tenant_name;index:idx_tenant_default;column:tenant_id" json:"tenant_id"`
	Name               string         `gorm:"type:varchar(100);not null;uniqueIndex:uk_tenant_name" json:"name"`
	Provider           ProviderType   `gorm:"type:varchar(20);default:'aws';column:provider" json:"provider"`
	Endpoint           string         `gorm:"type:varchar(255);not null" json:"endpoint"`
	AccessKey          string         `gorm:"type:varchar(255);not null;column:access_key" json:"-"`
	SecretKeyEncrypted string         `gorm:"type:text;not null;column:secret_key_encrypted" json:"-"`
	Bucket             string         `gorm:"type:varchar(100);not null" json:"bucket"`
	Region             string         `gorm:"type:varchar(50)" json:"region"`
	PathPrefix         string         `gorm:"type:varchar(255);default:'';column:path_prefix" json:"path_prefix"`
	Status             int8           `gorm:"type:tinyint;default:1" json:"status"`
	IsDefault          int8           `gorm:"type:tinyint;default:0;index:idx_tenant_default;column:is_default" json:"is_default"`
	CreatedAt          time.Time      `gorm:"precision:0" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"precision:0;autoUpdateTime" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

func (S3Config) TableName() string {
	return "s3_configs"
}

// DumplingTemplate Dumpling 参数模板
type DumplingTemplate struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    int64          `gorm:"not null;uniqueIndex:uk_tenant_name;column:tenant_id" json:"tenant_id"`
	Name        string         `gorm:"type:varchar(100);not null;uniqueIndex:uk_tenant_name" json:"name"`
	Threads     int            `gorm:"default:4" json:"threads"`
	RowsPerFile int            `gorm:"default:0;column:rows_per_file" json:"rows_per_file"`
	FileSize    string         `gorm:"type:varchar(20);default:'256MiB';column:file_size" json:"file_size"`
	Consistency string         `gorm:"type:varchar(20);default:auto" json:"consistency"`
	FilterRule  string         `gorm:"type:json;column:filter_rule" json:"filter_rule"`
	CreatedAt   time.Time      `gorm:"precision:0" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"precision:0;autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (DumplingTemplate) TableName() string {
	return "dumpling_templates"
}
