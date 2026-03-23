package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"claw-export-platform/config"
	"claw-export-platform/pkg/database"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func main() {
	// 解析命令行参数
	migrateOnly := flag.Bool("migrate", false, "run auto migrate and exit")
	rollback := flag.Bool("rollback", false, "rollback all tables")
	flag.Parse()

	// 加载配置
	cfg := config.Load()

	// 初始化日志
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logger.Sync()

	// 连接数据库
	dbCfg := database.Config{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		Database:        cfg.Database.Database,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	}

	db, err := database.Connect(dbCfg, logger)
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
		os.Exit(1)
	}

	// 验证连接
	if err := database.ValidateConnection(db); err != nil {
		logger.Fatal("database connection validation failed", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("database connected successfully",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("database", cfg.Database.Database),
	)

	if *rollback {
		logger.Warn("dropping all tables (rollback mode)")
		if err := rollbackTables(db); err != nil {
			logger.Fatal("rollback failed", zap.Error(err))
			os.Exit(1)
		}
		logger.Info("all tables dropped successfully")
		return
	}

	// 自动迁移
	logger.Info("running auto migration...")
	if err := database.AutoMigrate(db); err != nil {
		logger.Fatal("auto migration failed", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("auto migration completed successfully")

	if *migrateOnly {
		logger.Info("migrate-only mode, exiting")
		return
	}

	// 打印数据库统计
	stats := database.GetStats(db)
	logger.Info("database pool stats", zap.Any("stats", stats))

	fmt.Println("Migration completed successfully!")
}

func rollbackTables(db *gorm.DB) error {
	sqls := []string{
		"DROP TABLE IF EXISTS dumpling_templates",
		"DROP TABLE IF EXISTS audit_logs",
		"DROP TABLE IF EXISTS task_logs",
		"DROP TABLE IF EXISTS export_tasks",
		"DROP TABLE IF EXISTS s3_configs",
		"DROP TABLE IF EXISTS tidb_configs",
		"DROP TABLE IF EXISTS tenant_quotas",
		"DROP TABLE IF EXISTS tenants",
		"DROP TABLE IF EXISTS admins",
	}
	for _, sql := range sqls {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("failed to execute %q: %w", sql, err)
		}
	}
	return nil
}
