package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"claw-export-platform/api"
	"claw-export-platform/config"
	"claw-export-platform/pkg/database"
	"claw-export-platform/pkg/encryption"
	"claw-export-platform/pkg/queue"
	redispkg "claw-export-platform/pkg/redis"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	// 解析命令行参数
	port := flag.Int("port", 8080, "server port")
	mode := flag.String("mode", "release", "gin mode (debug/release)")
	flag.Parse()

	// 加载配置
	cfg := config.Load()

	// 初始化日志
	var logger *zap.Logger
	var err error
	if *mode == "debug" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 设置Gin模式
	gin.SetMode(*mode)

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

	// 创建Gin引擎
	engine := gin.New()

	// 设置路由
	router := api.NewRouter(db, taskQueue, encryptor, cfg, logger.Named("api"))
	router.Setup(engine)

	// 启动HTTP服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      engine,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting server", zap.Int("port", *port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server exited")
}
