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
	"claw-export-platform/services/cleanup"
	"claw-export-platform/services/task"
	"claw-export-platform/workers"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	workerCount := flag.Int("workers", cfg.Worker.Count, "number of workers")
	workDir := flag.String("work-dir", cfg.Worker.WorkDir, "working directory for export files")
	flag.Parse()

	cfg.Worker.Count = *workerCount
	cfg.Worker.WorkDir = *workDir
	if cfg.Redis.ConsumerName == "" {
		cfg.Redis.ConsumerName = buildConsumerName()
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	logger, err := newLogger(cfg.Server.Mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	db, err := database.Connect(database.Config{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		Database:        cfg.Database.Database,
		Charset:         cfg.Database.Charset,
		Loc:             cfg.Database.Loc,
		TLSMode:         cfg.Database.TLSMode,
		ServerName:      cfg.Database.ServerName,
		CAFile:          cfg.Database.CAFile,
		CertFile:        cfg.Database.CertFile,
		KeyFile:         cfg.Database.KeyFile,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime,
		DialTimeout:     cfg.Database.DialTimeout,
		ReadTimeout:     cfg.Database.ReadTimeout,
		WriteTimeout:    cfg.Database.WriteTimeout,
	}, logger)
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}
	defer database.Close(db)

	redisClient, err := redispkg.NewClient(redispkg.Config{
		Addr:               cfg.Redis.Addr,
		Username:           cfg.Redis.Username,
		Password:           cfg.Redis.Password,
		DB:                 cfg.Redis.DB,
		TLSEnabled:         cfg.Redis.TLSEnabled,
		InsecureSkipVerify: cfg.Redis.InsecureSkipVerify,
		ServerName:         cfg.Redis.ServerName,
		CAFile:             cfg.Redis.CAFile,
		CertFile:           cfg.Redis.CertFile,
		KeyFile:            cfg.Redis.KeyFile,
		DialTimeout:        cfg.Redis.DialTimeout,
		ReadTimeout:        cfg.Redis.ReadTimeout,
		WriteTimeout:       cfg.Redis.WriteTimeout,
		PoolSize:           cfg.Redis.PoolSize,
		MinIdleConns:       cfg.Redis.MinIdleConns,
		MaxRetries:         cfg.Redis.MaxRetries,
		StreamName:         cfg.Redis.StreamName,
		ConsumerGroup:      cfg.Redis.ConsumerGroup,
		ConsumerName:       cfg.Redis.ConsumerName,
		PendingTimeout:     cfg.Redis.PendingTimeout,
		BlockTimeout:       cfg.Redis.BlockTimeout,
	})
	if err != nil {
		logger.Fatal("failed to connect redis", zap.Error(err))
	}
	defer redisClient.Close()

	logger.Info("redis connected",
		zap.String("addr", cfg.Redis.Addr),
		zap.String("consumer_name", cfg.Redis.ConsumerName),
		zap.Bool("tls_enabled", cfg.Redis.TLSEnabled),
	)

	taskQueue := queue.NewQueue(redisClient, logger.Named("queue"))
	ctx := context.Background()
	if err := taskQueue.Initialize(ctx); err != nil {
		logger.Fatal("failed to initialize queue", zap.Error(err))
	}

	encryptor, err := encryption.NewEncryptor(cfg.Security.AESKey)
	if err != nil {
		logger.Fatal("failed to create encryptor", zap.Error(err))
	}

	if err := os.MkdirAll(cfg.Worker.WorkDir, 0755); err != nil {
		logger.Fatal("failed to create work directory", zap.Error(err))
	}

	workerPool := workers.NewWorkerPool(
		cfg.Worker.Count,
		db,
		taskQueue,
		encryptor,
		cfg.Worker.WorkDir,
		logger.Named("worker-pool"),
	)

	runtimeCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskManager := task.NewTaskManager(task.ManagerConfig{
		DB:                   db,
		Queue:                taskQueue,
		Logger:               logger.Named("task-manager"),
		TimeoutCheckInterval: cfg.Worker.TimeoutCheckInterval,
		TaskTimeout:          cfg.Worker.TaskTimeout,
	})
	go taskManager.StartTimeoutChecker(runtimeCtx)

	cleaner := cleanup.NewCleaner(cleanup.CleanerConfig{
		DB:            db,
		Encryptor:     encryptor,
		Logger:        logger.Named("cleaner"),
		CheckInterval: cfg.Worker.CleanupInterval,
		LogRetention:  cfg.Worker.LogRetention,
	})
	go cleaner.Start(runtimeCtx)

	logger.Info("starting worker pool",
		zap.Int("workers", cfg.Worker.Count),
		zap.String("work_dir", cfg.Worker.WorkDir),
	)
	workerPool.Start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	cancel()
	workerPool.Stop()
	logger.Info("worker shutdown complete")
}

func newLogger(mode string) (*zap.Logger, error) {
	if mode == gin.DebugMode {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}

func buildConsumerName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}
