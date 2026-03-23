package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 应用配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Security SecurityConfig `mapstructure:"security"`
}

// ServerConfig HTTP服务配置
type ServerConfig struct {
	Port int           `mapstructure:"port" default:"8080"`
	Mode string        `mapstructure:"mode" default:"release"` // debug / release
	Timeout time.Duration `mapstructure:"timeout" default:"30s"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string        `mapstructure:"host" default:"127.0.0.1"`
	Port            int           `mapstructure:"port" default:"4000"`
	User            string        `mapstructure:"user" default:"root"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database" default:"claw_export"`
	MaxOpenConns    int           `mapstructure:"max_open_conns" default:"50"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns" default:"10"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" default:"1h"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `mapstructure:"addr" default:"127.0.0.1:6379"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db" default:"0"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	AESKey          string `mapstructure:"aes_key"`
	JWTSecret       string `mapstructure:"jwt_secret"`
	TokenExpireHour int    `mapstructure:"token_expire_hour" default:"24"`
	BcryptCost      int    `mapstructure:"bcrypt_cost" default:"10"`
}

// Load 从环境变量加载配置
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:   getEnvInt("SERVER_PORT", 8080),
			Mode:   getEnv("SERVER_MODE", "release"),
			Timeout: getEnvDuration("SERVER_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "127.0.0.1"),
			Port:            getEnvInt("DB_PORT", 4000),
			User:            getEnv("DB_USER", "root"),
			Password:        getEnv("DB_PASSWORD", ""),
			Database:        getEnv("DB_NAME", "claw_export"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 50),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 1*time.Hour),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Security: SecurityConfig{
			AESKey:          getEnv("AES_KEY", ""),
			JWTSecret:       getEnv("JWT_SECRET", ""),
			TokenExpireHour: getEnvInt("JWT_TOKEN_EXPIRE_HOUR", 24),
			BcryptCost:      getEnvInt("BCRYPT_COST", 10),
		},
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return fallback
}

// Validate 验证配置完整性
func (c *Config) Validate() error {
	if c.Database.Password == "" {
		return fmt.Errorf("DB_PASSWORD is required")
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
	return nil
}
