package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 应用配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Security SecurityConfig `mapstructure:"security"`
	Worker   WorkerConfig   `mapstructure:"worker"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port            int           `mapstructure:"port" default:"8080"`
	Mode            string        `mapstructure:"mode" default:"release"`
	Timeout         time.Duration `mapstructure:"timeout" default:"30s"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" default:"10s"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string        `mapstructure:"host" default:"127.0.0.1"`
	Port            int           `mapstructure:"port" default:"4000"`
	User            string        `mapstructure:"user" default:"root"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database" default:"claw_export"`
	Charset         string        `mapstructure:"charset" default:"utf8mb4"`
	Loc             string        `mapstructure:"loc" default:"Local"`
	TLSMode         string        `mapstructure:"tls_mode" default:"disabled"`
	ServerName      string        `mapstructure:"server_name"`
	CAFile          string        `mapstructure:"ca_file"`
	CertFile        string        `mapstructure:"cert_file"`
	KeyFile         string        `mapstructure:"key_file"`
	MaxOpenConns    int           `mapstructure:"max_open_conns" default:"50"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" default:"10"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" default:"1h"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time" default:"10m"`
	DialTimeout     time.Duration `mapstructure:"dial_timeout" default:"10s"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout" default:"30s"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout" default:"30s"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr               string        `mapstructure:"addr" default:"127.0.0.1:6379"`
	Username           string        `mapstructure:"username"`
	Password           string        `mapstructure:"password"`
	DB                 int           `mapstructure:"db" default:"0"`
	TLSEnabled         bool          `mapstructure:"tls_enabled"`
	InsecureSkipVerify bool          `mapstructure:"insecure_skip_verify"`
	ServerName         string        `mapstructure:"server_name"`
	CAFile             string        `mapstructure:"ca_file"`
	CertFile           string        `mapstructure:"cert_file"`
	KeyFile            string        `mapstructure:"key_file"`
	DialTimeout        time.Duration `mapstructure:"dial_timeout" default:"5s"`
	ReadTimeout        time.Duration `mapstructure:"read_timeout" default:"10s"`
	WriteTimeout       time.Duration `mapstructure:"write_timeout" default:"10s"`
	PoolSize           int           `mapstructure:"pool_size" default:"20"`
	MinIdleConns       int           `mapstructure:"min_idle_conns" default:"5"`
	MaxRetries         int           `mapstructure:"max_retries" default:"3"`
	StreamName         string        `mapstructure:"stream_name" default:"export:tasks"`
	ConsumerGroup      string        `mapstructure:"consumer_group" default:"export-workers"`
	ConsumerName       string        `mapstructure:"consumer_name"`
	PendingTimeout     time.Duration `mapstructure:"pending_timeout" default:"30m"`
	BlockTimeout       time.Duration `mapstructure:"block_timeout" default:"5s"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	AESKey          string `mapstructure:"aes_key"`
	JWTSecret       string `mapstructure:"jwt_secret"`
	TokenExpireHour int    `mapstructure:"token_expire_hour" default:"24"`
	BcryptCost      int    `mapstructure:"bcrypt_cost" default:"10"`
}

// WorkerConfig Worker 运行配置
type WorkerConfig struct {
	Count                int           `mapstructure:"count" default:"4"`
	WorkDir              string        `mapstructure:"work_dir" default:"/tmp/exports"`
	TaskTimeout          time.Duration `mapstructure:"task_timeout" default:"2h"`
	TimeoutCheckInterval time.Duration `mapstructure:"timeout_check_interval" default:"1m"`
	CleanupInterval      time.Duration `mapstructure:"cleanup_interval" default:"1h"`
	LogRetention         time.Duration `mapstructure:"log_retention" default:"720h"`
}

// Load 从环境变量加载配置
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            getEnvInt([]string{"SERVER_PORT", "APP_PORT", "BACKEND_PORT"}, 8080),
			Mode:            normalizeServerMode(getEnv([]string{"SERVER_MODE", "APP_ENV"}, "release")),
			Timeout:         getEnvDuration([]string{"SERVER_TIMEOUT"}, 30*time.Second),
			ShutdownTimeout: getEnvDuration([]string{"SERVER_SHUTDOWN_TIMEOUT"}, 10*time.Second),
		},
		Database: DatabaseConfig{
			Host:            getEnv([]string{"DB_HOST"}, "127.0.0.1"),
			Port:            getEnvInt([]string{"DB_PORT"}, 4000),
			User:            getEnv([]string{"DB_USER", "MYSQL_USER"}, "root"),
			Password:        getEnv([]string{"DB_PASSWORD", "MYSQL_PASSWORD"}, ""),
			Database:        getEnv([]string{"DB_NAME", "MYSQL_DATABASE"}, "claw_export"),
			Charset:         getEnv([]string{"DB_CHARSET"}, "utf8mb4"),
			Loc:             getEnv([]string{"DB_LOC"}, "Local"),
			TLSMode:         normalizeTLSMode(getEnv([]string{"DB_TLS_MODE"}, "disabled")),
			ServerName:      getEnv([]string{"DB_TLS_SERVER_NAME"}, ""),
			CAFile:          getEnv([]string{"DB_TLS_CA_FILE"}, ""),
			CertFile:        getEnv([]string{"DB_TLS_CERT_FILE"}, ""),
			KeyFile:         getEnv([]string{"DB_TLS_KEY_FILE"}, ""),
			MaxOpenConns:    getEnvInt([]string{"DB_MAX_OPEN_CONNS"}, 50),
			MaxIdleConns:    getEnvInt([]string{"DB_MAX_IDLE_CONNS"}, 10),
			ConnMaxLifetime: getEnvDuration([]string{"DB_CONN_MAX_LIFETIME"}, time.Hour),
			ConnMaxIdleTime: getEnvDuration([]string{"DB_CONN_MAX_IDLE_TIME"}, 10*time.Minute),
			DialTimeout:     getEnvDuration([]string{"DB_DIAL_TIMEOUT"}, 10*time.Second),
			ReadTimeout:     getEnvDuration([]string{"DB_READ_TIMEOUT"}, 30*time.Second),
			WriteTimeout:    getEnvDuration([]string{"DB_WRITE_TIMEOUT"}, 30*time.Second),
		},
		Redis: RedisConfig{
			Addr:               getEnv([]string{"REDIS_ADDR"}, "127.0.0.1:6379"),
			Username:           getEnv([]string{"REDIS_USERNAME"}, ""),
			Password:           getEnv([]string{"REDIS_PASSWORD"}, ""),
			DB:                 getEnvInt([]string{"REDIS_DB"}, 0),
			TLSEnabled:         getEnvBool([]string{"REDIS_TLS_ENABLED", "REDIS_TLS"}, false),
			InsecureSkipVerify: getEnvBool([]string{"REDIS_TLS_INSECURE_SKIP_VERIFY"}, false),
			ServerName:         getEnv([]string{"REDIS_TLS_SERVER_NAME"}, ""),
			CAFile:             getEnv([]string{"REDIS_TLS_CA_FILE"}, ""),
			CertFile:           getEnv([]string{"REDIS_TLS_CERT_FILE"}, ""),
			KeyFile:            getEnv([]string{"REDIS_TLS_KEY_FILE"}, ""),
			DialTimeout:        getEnvDuration([]string{"REDIS_DIAL_TIMEOUT"}, 5*time.Second),
			ReadTimeout:        getEnvDuration([]string{"REDIS_READ_TIMEOUT"}, 10*time.Second),
			WriteTimeout:       getEnvDuration([]string{"REDIS_WRITE_TIMEOUT"}, 10*time.Second),
			PoolSize:           getEnvInt([]string{"REDIS_POOL_SIZE"}, 20),
			MinIdleConns:       getEnvInt([]string{"REDIS_MIN_IDLE_CONNS"}, 5),
			MaxRetries:         getEnvInt([]string{"REDIS_MAX_RETRIES"}, 3),
			StreamName:         getEnv([]string{"REDIS_STREAM_NAME"}, "export:tasks"),
			ConsumerGroup:      getEnv([]string{"REDIS_CONSUMER_GROUP"}, "export-workers"),
			ConsumerName:       getEnv([]string{"REDIS_CONSUMER_NAME"}, ""),
			PendingTimeout:     getEnvDuration([]string{"REDIS_PENDING_TIMEOUT"}, 30*time.Minute),
			BlockTimeout:       getEnvDuration([]string{"REDIS_BLOCK_TIMEOUT"}, 5*time.Second),
		},
		Security: SecurityConfig{
			AESKey:          getEnv([]string{"AES_KEY", "ENCRYPTION_KEY"}, ""),
			JWTSecret:       getEnv([]string{"JWT_SECRET"}, ""),
			TokenExpireHour: getEnvInt([]string{"JWT_TOKEN_EXPIRE_HOUR"}, 24),
			BcryptCost:      getEnvInt([]string{"BCRYPT_COST"}, 10),
		},
		Worker: WorkerConfig{
			Count:                getEnvInt([]string{"WORKER_COUNT"}, 4),
			WorkDir:              getEnv([]string{"WORK_DIR"}, "/tmp/exports"),
			TaskTimeout:          getEnvDuration([]string{"WORKER_TASK_TIMEOUT"}, 2*time.Hour),
			TimeoutCheckInterval: getEnvDuration([]string{"WORKER_TIMEOUT_CHECK_INTERVAL"}, time.Minute),
			CleanupInterval:      getEnvDuration([]string{"WORKER_CLEANUP_INTERVAL"}, time.Hour),
			LogRetention:         getEnvDuration([]string{"WORKER_LOG_RETENTION"}, 30*24*time.Hour),
		},
	}
}

func getEnv(keys []string, fallback string) string {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return fallback
}

func getEnvInt(keys []string, fallback int) int {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			intValue, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil {
				return intValue
			}
		}
	}
	return fallback
}

func getEnvDuration(keys []string, fallback time.Duration) time.Duration {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			duration, err := time.ParseDuration(strings.TrimSpace(value))
			if err == nil {
				return duration
			}
		}
	}
	return fallback
}

func getEnvBool(keys []string, fallback bool) bool {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			parsed, err := strconv.ParseBool(strings.TrimSpace(value))
			if err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func normalizeServerMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug", "development", "dev":
		return "debug"
	default:
		return "release"
	}
}

func normalizeTLSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "disable", "disabled", "false", "off":
		return "disabled"
	case "preferred":
		return "preferred"
	case "skip-verify":
		return "skip-verify"
	case "require", "required", "true", "on":
		return "required"
	case "verify-ca", "verify_ca":
		return "verify-ca"
	case "verify-identity", "verify_identity":
		return "verify-identity"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// Validate 验证配置完整性
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Database.Host) == "" {
		return fmt.Errorf("DB_HOST is required")
	}
	if strings.TrimSpace(c.Database.User) == "" {
		return fmt.Errorf("DB_USER is required")
	}
	if c.Database.Password == "" {
		return fmt.Errorf("DB_PASSWORD is required")
	}
	if strings.TrimSpace(c.Database.Database) == "" {
		return fmt.Errorf("DB_NAME is required")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}
	if c.Security.AESKey == "" {
		return fmt.Errorf("AES_KEY is required (must be 32 bytes for AES-256)")
	}
	if len(c.Security.AESKey) != 32 {
		return fmt.Errorf("AES_KEY must be exactly 32 bytes, got %d", len(c.Security.AESKey))
	}
	if c.Security.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if c.Worker.Count <= 0 {
		return fmt.Errorf("WORKER_COUNT must be greater than 0")
	}
	if strings.TrimSpace(c.Worker.WorkDir) == "" {
		return fmt.Errorf("WORK_DIR is required")
	}
	return nil
}
