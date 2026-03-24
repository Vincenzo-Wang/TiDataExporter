package export

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"claw-export-platform/models"
	"claw-export-platform/pkg/encryption"
	"claw-export-platform/services/s3"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Executor 导出执行器
type Executor struct {
	db        *gorm.DB
	s3Client  *s3.Client
	encryptor *encryption.Encryptor
	logger    *zap.Logger
	workDir   string
}

// NewExecutor 创建执行器
func NewExecutor(db *gorm.DB, s3Client *s3.Client, encryptor *encryption.Encryptor, workDir string, logger *zap.Logger) *Executor {
	return &Executor{
		db:        db,
		s3Client:  s3Client,
		encryptor: encryptor,
		logger:    logger,
		workDir:   workDir,
	}
}

// Execute 执行导出任务
func (e *Executor) Execute(ctx context.Context, taskID int64, tidbConfig *models.TiDBConfig, sqlText, filetype, compress string) (*ExecutionResult, error) {
	// 解密密码
	password, err := e.encryptor.Decrypt(tidbConfig.PasswordEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt tidb password: %w", err)
	}

	// 创建工作目录
	taskDir := filepath.Join(e.workDir, fmt.Sprintf("task_%d", taskID))
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create task directory: %w", err)
	}
	defer os.RemoveAll(taskDir) // 清理临时目录

	// 构建输出文件名
	outputFile := filepath.Join(taskDir, fmt.Sprintf("output.%s", filetype))
	s3Key := fmt.Sprintf("exports/%d/output.%s", taskID, filetype)
	if compress != "" {
		s3Key += "." + compress
	}

	// 构建Dumpling命令
	cmd := e.buildDumplingCommand(tidbConfig, password, sqlText, filetype, compress, outputFile, taskDir)

	e.logger.Info("executing dumpling",
		zap.Int64("task_id", taskID),
		zap.String("host", tidbConfig.Host),
		zap.Int("port", tidbConfig.Port),
		zap.String("database", tidbConfig.Database),
	)

	// 执行命令
	startTime := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	if err != nil {
		e.logTaskError(ctx, taskID, string(output), err)
		return nil, fmt.Errorf("dumpling failed: %w, output: %s", err, string(output))
	}

	// 获取输出文件信息
	fileInfo, err := os.Stat(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}
	fileSize := fileInfo.Size()

	// 上传到S3
	if e.s3Client != nil {
		file, err := os.Open(outputFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open output file: %w", err)
		}
		defer file.Close()

		contentType := "application/octet-stream"
		if filetype == "csv" {
			contentType = "text/csv"
		}

		if err := e.s3Client.Upload(ctx, s3Key, file, fileSize, contentType); err != nil {
			return nil, fmt.Errorf("failed to upload to s3: %w", err)
		}
	}

	e.logger.Info("export completed",
		zap.Int64("task_id", taskID),
		zap.Int64("file_size", fileSize),
		zap.Duration("duration", duration),
	)

	return &ExecutionResult{
		FileURL:  s3Key,
		FileSize: fileSize,
		Duration: duration,
	}, nil
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	FileURL  string
	FileSize int64
	Duration time.Duration
}

func (e *Executor) buildDumplingCommand(tidbConfig *models.TiDBConfig, password, sqlText, filetype, compress, outputFile, workDir string) *exec.Cmd {
	dumplingPath := strings.TrimSpace(os.Getenv("DUMPLING_PATH"))
	if dumplingPath == "" {
		dumplingPath = "/usr/local/bin/dumpling"
	}

	args := []string{
		fmt.Sprintf("--host=%s", tidbConfig.Host),
		fmt.Sprintf("--port=%d", tidbConfig.Port),
		fmt.Sprintf("--user=%s", tidbConfig.Username),
		fmt.Sprintf("--password=%s", password),
		fmt.Sprintf("--output=%s", workDir),
		fmt.Sprintf("--sql=%s", sqlText),
	}

	if tidbConfig.Database != "" {
		args = append(args, fmt.Sprintf("--database=%s", tidbConfig.Database))
	}

	// 文件类型
	if filetype == "csv" {
		args = append(args, "--filetype=csv")
	} else {
		args = append(args, "--filetype=sql")
	}

	// 压缩
	if compress != "" {
		args = append(args, fmt.Sprintf("--compress=%s", compress))
	}

	// 其他默认参数
	args = append(args,
		"--threads=4",
		"--rows=100000",
		"--consistency=auto",
	)

	return exec.Command(dumplingPath, args...)
}

func (e *Executor) logTaskError(ctx context.Context, taskID int64, output string, err error) {
	e.logger.Error("task execution failed",
		zap.Int64("task_id", taskID),
		zap.String("output", output),
		zap.Error(err),
	)

	// 记录到task_logs表
	logEntry := &models.TaskLog{
		TaskID:   taskID,
		LogLevel: "ERROR",
		Message:  fmt.Sprintf("Execution failed: %s\nOutput: %s", err.Error(), truncate(output, 10000)),
	}
	e.db.WithContext(ctx).Create(logEntry)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ValidateSQL 验证SQL语句安全性
func ValidateSQL(sqlText string) error {
	sqlUpper := strings.ToUpper(sqlText)
	
	// 危险关键字检查
	dangerousKeywords := []string{
		"DROP", "DELETE", "TRUNCATE", "ALTER", "CREATE", "INSERT", "UPDATE",
		"GRANT", "REVOKE", "EXEC", "EXECUTE", "CALL",
	}

	for _, keyword := range dangerousKeywords {
		// 简单检查，实际应使用SQL解析器
		if strings.Contains(sqlUpper, keyword+" ") || strings.Contains(sqlUpper, keyword+"\n") {
			return fmt.Errorf("SQL contains forbidden keyword: %s", keyword)
		}
	}

	return nil
}
