package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"claw-export-platform/config"
	"claw-export-platform/pkg/database"
	"claw-export-platform/pkg/encryption"
	"claw-export-platform/pkg/queue"
	redispkg "claw-export-platform/pkg/redis"
	"claw-export-platform/workers"

	"go.uber.org/zap"
)

func main() {
	// 解析命令行参数
	workerCount := flag.Int("workers", 4, "number of workers")
	workDir := flag.String("work-dir", "/tmp/exports", "working directory for export files")
	flag.Parse()

	// 加载配置
	cfg := config.Load()

	// 初始化日志
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
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
	defer database.Close(db)

	logger.Info("database connected",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("database", cfg.Database.Database),
	)

	// 连接Redis
	redisClient, err := redispkg.NewClient(redispkg.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		logger.Fatal("failed to connect redis", zap.Error(err))
		os.Exit(1)
	}
	defer redisClient.Close()

	logger.Info("redis connected", zap.String("addr", cfg.Redis.Addr))

	// 初始化任务队列
	taskQueue := queue.NewQueue(redisClient, logger.Named("queue"))
	ctx := context.Background()
	if err := taskQueue.Initialize(ctx); err != nil {
		logger.Fatal("failed to initialize queue", zap.Error(err))
		os.Exit(1)
	}

	// 创建加密器
	encryptor, err := encryption.NewEncryptor(cfg.Security.AESKey)
	if err != nil {
		logger.Fatal("failed to create encryptor", zap.Error(err))
		os.Exit(1)
	}

	// 确保工作目录存在
	if err := os.MkdirAll(*workDir, 0755); err != nil {
		logger.Fatal("failed to create work directory", zap.Error(err))
		os.Exit(1)
	}

	// 创建Worker池
	workerPool := workers.NewWorkerPool(
		*workerCount,
		db,
		taskQueue,
		encryptor,
		*workDir,
		logger.Named("worker-pool"),
	)

	// 启动Worker池
	logger.Info("starting worker pool", zap.Int("workers", *workerCount))
	workerPool.Start()

	// 等待终止信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// 优雅关闭
	workerPool.Stop()
	logger.Info("worker shutdown complete")
}
