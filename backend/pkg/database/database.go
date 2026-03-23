package database

import (
	"context"
	"fmt"
	"time"

	"claw-export-platform/models"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config 数据库配置
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DB 全局数据库连接
var DB *gorm.DB

// Connect 建立数据库连接
func Connect(cfg Config, logger *zap.Logger) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=10s&readTimeout=30s&writeTimeout=30s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	gormConfig := &gorm.Config{
		Logger:                 newGormLogger(logger),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	}

	db, err := gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	DB = db
	return db, nil
}

// AutoMigrate 自动迁移所有表结构
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Admin{},
		&models.Tenant{},
		&models.TenantQuota{},
		&models.TiDBConfig{},
		&models.S3Config{},
		&models.ExportTask{},
		&models.TaskLog{},
		&models.AuditLog{},
		&models.DumplingTemplate{},
	)
}

// Close 关闭数据库连接
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// gormLogger 适配zap到gorm logger
type gormLogger struct {
	logger *zap.Logger
}

func newGormLogger(logger *zap.Logger) *gormLogger {
	return &gormLogger{logger: logger.Named("gorm")}
}

func (l *gormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l *gormLogger) Info(_ context.Context, _ string, _ ...interface{}) {
}

func (l *gormLogger) Warn(_ context.Context, _ string, _ ...interface{}) {
}

func (l *gormLogger) Error(_ context.Context, _ string, _ ...interface{}) {
}

func (l *gormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	if err != nil {
		l.logger.Error("sql error",
			zap.Error(err),
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
		return
	}

	if elapsed > 200*time.Millisecond {
		l.logger.Warn("slow sql",
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
	}
}

// ValidateConnection 验证数据库连接
func ValidateConnection(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// GetStats 获取数据库连接池统计
func GetStats(db *gorm.DB) map[string]interface{} {
	sqlDB, err := db.DB()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	stats := sqlDB.Stats()
	return map[string]interface{}{
		"max_open_connections": stats.MaxOpenConnections,
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration":        stats.WaitDuration.String(),
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_idle_time_closed": stats.MaxIdleTimeClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}
}
