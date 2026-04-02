package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"claw-export-platform/models"

	drivermysql "github.com/go-sql-driver/mysql"
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
	Charset         string
	Loc             string
	TLSMode         string
	ServerName      string
	CAFile          string
	CertFile        string
	KeyFile         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
}

// DB 全局数据库连接
var DB *gorm.DB

// Connect 建立数据库连接
func Connect(cfg Config, logger *zap.Logger) (*gorm.DB, error) {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}

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
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	DB = db
	return db, nil
}

func buildDSN(cfg Config) (string, error) {
	loc := time.Local
	if strings.TrimSpace(cfg.Loc) != "" && !strings.EqualFold(cfg.Loc, "local") {
		loadedLoc, err := time.LoadLocation(cfg.Loc)
		if err != nil {
			return "", fmt.Errorf("failed to load DB_LOC %q: %w", cfg.Loc, err)
		}
		loc = loadedLoc
	}

	driverCfg := drivermysql.NewConfig()
	driverCfg.User = cfg.User
	driverCfg.Passwd = cfg.Password
	driverCfg.Net = "tcp"
	driverCfg.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	driverCfg.DBName = cfg.Database
	driverCfg.Params = map[string]string{}
	driverCfg.ParseTime = true
	driverCfg.Loc = loc
	driverCfg.Timeout = cfg.DialTimeout
	driverCfg.ReadTimeout = cfg.ReadTimeout
	driverCfg.WriteTimeout = cfg.WriteTimeout
	if strings.TrimSpace(cfg.Charset) != "" {
		driverCfg.Params["charset"] = cfg.Charset
	}

	tlsName, err := resolveTLSConfig(cfg)
	if err != nil {
		return "", err
	}
	if tlsName != "" {
		driverCfg.TLSConfig = tlsName
	}

	return driverCfg.FormatDSN(), nil
}

func resolveTLSConfig(cfg Config) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	switch mode {
	case "", "disable", "disabled", "false", "off":
		return "", nil
	case "preferred":
		return "preferred", nil
	case "skip-verify":
		return "skip-verify", nil
	}

	hasCustomFiles := strings.TrimSpace(cfg.CAFile) != "" || strings.TrimSpace(cfg.CertFile) != "" || strings.TrimSpace(cfg.KeyFile) != "" || strings.TrimSpace(cfg.ServerName) != ""
	if !hasCustomFiles {
		switch mode {
		case "required", "require", "true", "on":
			return "true", nil
		case "verify-ca", "verify_identity", "verify-identity", "verify_ca":
			// 使用自定义 TLS 配置来显式开启校验。
		default:
			return "true", nil
		}
	}

	tlsConfig, err := buildCustomTLSConfig(cfg, mode)
	if err != nil {
		return "", err
	}

	name := buildTLSRegistrationName(cfg)
	if err := drivermysql.RegisterTLSConfig(name, tlsConfig); err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("failed to register DB TLS config: %w", err)
	}

	return name, nil
}

func buildCustomTLSConfig(cfg Config, mode string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	switch mode {
	case "required", "require":
		tlsConfig.InsecureSkipVerify = true
	case "verify-identity", "verify_identity":
		tlsConfig.ServerName = firstNonEmpty(cfg.ServerName, cfg.Host)
	}

	if strings.TrimSpace(cfg.CAFile) != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read DB TLS CA file: %w", err)
		}
		rootCAs, err := x509.SystemCertPool()
		if err != nil || rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}
		if !rootCAs.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse DB TLS CA file")
		}
		tlsConfig.RootCAs = rootCAs
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = firstNonEmpty(cfg.ServerName, cfg.Host)
		}
	}

	if strings.TrimSpace(cfg.CertFile) != "" || strings.TrimSpace(cfg.KeyFile) != "" {
		if strings.TrimSpace(cfg.CertFile) == "" || strings.TrimSpace(cfg.KeyFile) == "" {
			return nil, fmt.Errorf("DB_TLS_CERT_FILE and DB_TLS_KEY_FILE must be configured together")
		}
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load DB TLS client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

func buildTLSRegistrationName(cfg Config) string {
	replacer := strings.NewReplacer(".", "_", ":", "_", "-", "_")
	return fmt.Sprintf("claw_db_%s_%d_%s", replacer.Replace(cfg.Host), cfg.Port, replacer.Replace(strings.ToLower(cfg.TLSMode)))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
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

// gormLogger 适配 zap 到 gorm logger
type gormLogger struct {
	logger *zap.Logger
}

func newGormLogger(logger *zap.Logger) *gormLogger {
	return &gormLogger{logger: logger.Named("gorm")}
}

func (l *gormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l *gormLogger) Info(_ context.Context, _ string, _ ...interface{}) {}

func (l *gormLogger) Warn(_ context.Context, _ string, _ ...interface{}) {}

func (l *gormLogger) Error(_ context.Context, _ string, _ ...interface{}) {}

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
