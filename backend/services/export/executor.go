package export

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	encryptor *encryption.Encryptor
	logger    *zap.Logger
	workDir   string
}

// NewExecutor 创建执行器
func NewExecutor(db *gorm.DB, encryptor *encryption.Encryptor, workDir string, logger *zap.Logger) *Executor {
	return &Executor{
		db:        db,
		encryptor: encryptor,
		logger:    logger,
		workDir:   workDir,
	}
}

// Execute 执行导出任务
func (e *Executor) Execute(ctx context.Context, taskID, tenantID int64, bizName, taskName string, tidbConfig *models.TiDBConfig, s3Config *models.S3Config, sqlText, filetype, compress string) (*ExecutionResult, error) {
	// 解密S3密钥
	secretKey, err := e.encryptor.Decrypt(s3Config.SecretKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt s3 secret key: %w", err)
	}

	// 创建S3客户端
	s3Client, err := s3.NewStorageClient(ctx, s3.Config{
		Provider:   string(s3Config.Provider),
		Endpoint:   s3Config.Endpoint,
		AccessKey:  s3Config.AccessKey,
		SecretKey:  secretKey,
		Bucket:     s3Config.Bucket,
		Region:     s3Config.Region,
		PathPrefix: s3Config.PathPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create s3 client: %w", err)
	}
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
	rowCount := parseDumplingRowCount(string(output))

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

	sort.Strings(files)
	e.logger.Info("found output files",
		zap.Int64("task_id", taskID),
		zap.Int("total_files", len(files)),
	)

	contentType := "application/octet-stream"
	if filetype == "csv" {
		contentType = "text/csv"
	}

	resolvedBizName := resolveBizName(bizName, taskName)
	bizSlug := normalizeBizSlug(resolvedBizName)

	var totalFileSize int64
	resultFiles := make([]ExecutionFile, 0, len(files))

	for i, outputFile := range files {
		fileInfo, err := os.Stat(outputFile)
		if err != nil {
			return nil, fmt.Errorf("failed to stat output file %s: %w", outputFile, err)
		}
		fileSize := fileInfo.Size()
		totalFileSize += fileSize

		ext := buildOutputFileExt(outputFile, filetype, compress)
		s3Key := fmt.Sprintf("exports/%d/%s/%d/part_%06d%s", tenantID, bizSlug, taskID, i+1, ext)

		if s3Client != nil {
			file, err := os.Open(outputFile)
			if err != nil {
				return nil, fmt.Errorf("failed to open output file %s: %w", outputFile, err)
			}

			if err := s3Client.Upload(ctx, s3Key, file, fileSize, contentType); err != nil {
				file.Close()
				return nil, fmt.Errorf("failed to upload %s to s3: %w", outputFile, err)
			}
			file.Close()
		}

		resultFiles = append(resultFiles, ExecutionFile{
			Path:    s3Key,
			Name:    buildDisplayName(resolvedBizName, taskID, i+1, ext),
			RawName: filepath.Base(outputFile),
			Size:    fileSize,
		})
	}

	e.logger.Info("export completed",
		zap.Int64("task_id", taskID),
		zap.Int64("file_size", totalFileSize),
		zap.Int("file_count", len(resultFiles)),
		zap.Duration("duration", duration),
	)

	return &ExecutionResult{
		FileURL:  resultFiles[0].Path,
		FileSize: totalFileSize,
		RowCount: rowCount,
		Files:    resultFiles,
		Duration: duration,
	}, nil
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	FileURL  string
	FileSize int64
	RowCount int64
	Files    []ExecutionFile
	Duration time.Duration
}

// ExecutionFile 导出结果文件
// Path 为对象存储中的 key，Name 为下载展示名，RawName 为本地原始名，Size 为文件大小（字节）
type ExecutionFile struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	RawName string `json:"raw_name"`
	Size    int64  `json:"size"`
}

func buildOutputFileExt(outputFile, filetype, compress string) string {
	baseName := filepath.Base(outputFile)
	if compress != "" && strings.HasSuffix(baseName, "."+compress) {
		nameWithoutCompress := strings.TrimSuffix(baseName, "."+compress)
		nameExt := filepath.Ext(nameWithoutCompress)
		if nameExt != "" {
			return nameExt + "." + compress
		}
		return "." + filetype + "." + compress
	}

	ext := filepath.Ext(baseName)
	if ext != "" {
		return ext
	}

	if compress != "" {
		return "." + filetype + "." + compress
	}
	return "." + filetype
}

func parseDumplingRowCount(output string) int64 {
	if strings.TrimSpace(output) == "" {
		return 0
	}

	patterns := []string{
		`(?i)total\s+rows\s*[:=]\s*([0-9][0-9,]*)`,
		`(?i)rows\s*[:=]\s*([0-9][0-9,]*)`,
		`(?i)\(([0-9][0-9,]*)\s+rows\)`,
	}

	var maxRows int64
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(output, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			normalized := strings.ReplaceAll(m[1], ",", "")
			rows, err := strconv.ParseInt(normalized, 10, 64)
			if err != nil || rows < 0 {
				continue
			}
			if rows > maxRows {
				maxRows = rows
			}
		}
	}
	return maxRows
}

func resolveBizName(bizName, taskName string) string {
	if v := strings.TrimSpace(bizName); v != "" {
		return v
	}
	if v := strings.TrimSpace(taskName); v != "" {
		return v
	}
	return "export"
}

func normalizeBizSlug(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == '-':
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "export"
	}
	runes := []rune(slug)
	if len(runes) > 40 {
		slug = strings.Trim(string(runes[:40]), "-")
	}
	if slug == "" {
		return "export"
	}
	return slug
}

func buildDisplayName(bizName string, taskID int64, index int, ext string) string {
	name := strings.TrimSpace(bizName)
	if name == "" {
		name = "export"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.Join(strings.Fields(name), "_")
	if name == "" {
		name = "export"
	}
	suffix := fmt.Sprintf("_%d_part%03d%s", taskID, index, ext)
	maxNameRunes := 128 - len([]rune(suffix))
	if maxNameRunes < 1 {
		maxNameRunes = 1
	}
	nameRunes := []rune(name)
	if len(nameRunes) > maxNameRunes {
		name = string(nameRunes[:maxNameRunes])
	}
	return name + suffix
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

	if filetype == "csv" {
		args = append(args, "--filetype=csv")
	} else {
		args = append(args, "--filetype=sql")
	}

	if compress != "" {
		args = append(args, fmt.Sprintf("--compress=%s", compress))
	}

	args = append(args, e.buildDumplingTLSArgs(tidbConfig)...)

	threads := getPositiveIntEnv("DUMPLING_THREADS", 8)
	rows := getPositiveIntEnv("DUMPLING_ROWS", 200000)
	consistency := getNonEmptyEnv("DUMPLING_CONSISTENCY", "auto")
	fileSize := getNonEmptyEnv("DUMPLING_FILE_SIZE", "100M")

	args = append(args,
		fmt.Sprintf("--threads=%d", threads),
		fmt.Sprintf("--rows=%d", rows),
		fmt.Sprintf("--consistency=%s", consistency),
		fmt.Sprintf("-F=%s", fileSize),
	)

	return exec.Command(dumplingPath, args...)
}

func (e *Executor) buildDumplingTLSArgs(tidbConfig *models.TiDBConfig) []string {
	sslMode := strings.ToLower(strings.TrimSpace(tidbConfig.SSLMode))
	if sslMode == "" || sslMode == "disabled" {
		return nil
	}

	var args []string
	if tidbConfig.SSLCA != "" {
		args = append(args, fmt.Sprintf("--ca=%s", tidbConfig.SSLCA))
	}
	if tidbConfig.SSLCert != "" {
		args = append(args, fmt.Sprintf("--cert=%s", tidbConfig.SSLCert))
	}
	if tidbConfig.SSLKey != "" {
		args = append(args, fmt.Sprintf("--key=%s", tidbConfig.SSLKey))
	}

	if len(args) == 0 {
		e.logger.Warn("tidb ssl_mode is enabled but no dumpling TLS files are configured",
			zap.String("ssl_mode", tidbConfig.SSLMode),
			zap.String("host", tidbConfig.Host),
		)
	}

	return args
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

func getNonEmptyEnv(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func getPositiveIntEnv(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
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
