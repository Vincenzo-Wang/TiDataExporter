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

	// 构建S3 key
	s3Key := fmt.Sprintf("exports/%d/output.%s", taskID, filetype)
	if compress != "" {
		s3Key += "." + compress
	}

	// 构建Dumpling命令 - output 是目录，不是文件
	cmd := e.buildDumplingCommand(tidbConfig, password, sqlText, filetype, compress, taskDir)

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

	// 记录 dumpling 输出（用于调试）
	e.logger.Info("dumpling output",
		zap.Int64("task_id", taskID),
		zap.String("output", string(output)),
	)

	if err != nil {
		e.logTaskError(ctx, taskID, string(output), err)
		return nil, fmt.Errorf("dumpling failed: %w, output: %s", err, string(output))
	}

	// dumpling 输出到目录，需要查找实际的输出文件
	// dumpling 的文件命名格式: {database}.{table}.{filetype} 或类似格式
	files, err := filepath.Glob(filepath.Join(taskDir, "*."+filetype))
	if err != nil {
		return nil, fmt.Errorf("failed to find output files: %w", err)
	}

	// 如果没找到 .csv/.sql 文件，尝试查找压缩文件
	if len(files) == 0 && compress != "" {
		files, err = filepath.Glob(filepath.Join(taskDir, "*."+filetype+"."+compress))
		if err != nil {
			return nil, fmt.Errorf("failed to find compressed output files: %w", err)
		}
	}

	// 如果还是没找到，列出目录中所有文件
	if len(files) == 0 {
		entries, listErr := os.ReadDir(taskDir)
		if listErr != nil {
			return nil, fmt.Errorf("no output files found, failed to list directory: %w", listErr)
		}
		for _, entry := range entries {
			files = append(files, filepath.Join(taskDir, entry.Name()))
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no output files found in %s", taskDir)
	}

	// 如果只有一个文件，直接使用
	// 如果有多个文件，目前只使用第一个（后续可以考虑打包）
	outputFile := files[0]
	e.logger.Info("found output file",
		zap.Int64("task_id", taskID),
		zap.String("file", outputFile),
		zap.Int("total_files", len(files)),
	)

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

		// 使用实际文件名作为 S3 key
		actualS3Key := fmt.Sprintf("exports/%d/%s", taskID, filepath.Base(outputFile))
		if err := e.s3Client.Upload(ctx, actualS3Key, file, fileSize, contentType); err != nil {
			return nil, fmt.Errorf("failed to upload to s3: %w", err)
		}
		s3Key = actualS3Key
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

func (e *Executor) buildDumplingCommand(tidbConfig *models.TiDBConfig, password, sqlText, filetype, compress, outputDir string) *exec.Cmd {
	dumplingPath := strings.TrimSpace(os.Getenv("DUMPLING_PATH"))
	if dumplingPath == "" {
		dumplingPath = "/usr/local/bin/dumpling"
	}

	args := []string{
		fmt.Sprintf("--host=%s", tidbConfig.Host),
		fmt.Sprintf("--port=%d", tidbConfig.Port),
		fmt.Sprintf("--user=%s", tidbConfig.Username),
		fmt.Sprintf("--password=%s", password),
		fmt.Sprintf("--output=%s", outputDir),
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
