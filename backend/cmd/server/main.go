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
	cfg := config.Load()

	port := flag.Int("port", cfg.Server.Port, "server port")
	mode := flag.String("mode", cfg.Server.Mode, "gin mode (debug/release)")
	flag.Parse()

	cfg.Server.Port = *port
	cfg.Server.Mode = *mode

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

	gin.SetMode(cfg.Server.Mode)

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

	logger.Info("database connected",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("database", cfg.Database.Database),
		zap.String("tls_mode", cfg.Database.TLSMode),
	)

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

	engine := gin.New()
	router := api.NewRouter(db, taskQueue, encryptor, cfg, logger.Named("api"))
	router.Setup(engine)

	idleTimeout := cfg.Server.Timeout * 2
	if idleTimeout < cfg.Server.Timeout {
		idleTimeout = cfg.Server.Timeout
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      engine,
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout,
		IdleTimeout:  idleTimeout,
	}

	go func() {
		logger.Info("starting server",
			zap.Int("port", cfg.Server.Port),
			zap.String("mode", cfg.Server.Mode),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server exited")
}

func newLogger(mode string) (*zap.Logger, error) {
	if mode == gin.DebugMode {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}
