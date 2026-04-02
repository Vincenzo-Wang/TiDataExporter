package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"claw-export-platform/config"
	"claw-export-platform/pkg/database"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const migrationTableName = "schema_migrations"

type appliedMigration struct {
	Version  string
	Checksum string
}

func main() {
	action := flag.String("action", "up", "migration action: up or status")
	migrationDir := flag.String("dir", "", "migration directory")
	flag.Parse()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logger.Sync()

	dir, err := resolveMigrationDir(*migrationDir)
	if err != nil {
		logger.Fatal("failed to resolve migration directory", zap.Error(err))
	}

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

	if err := database.ValidateConnection(db); err != nil {
		logger.Fatal("database connection validation failed", zap.Error(err))
	}

	if err := ensureMigrationTable(db); err != nil {
		logger.Fatal("failed to ensure migration table", zap.Error(err))
	}

	switch strings.ToLower(strings.TrimSpace(*action)) {
	case "up":
		if err := applyMigrations(db, dir, logger); err != nil {
			logger.Fatal("migration failed", zap.Error(err))
		}
		logger.Info("migration completed successfully", zap.String("dir", dir))
	case "status":
		if err := printMigrationStatus(db, dir); err != nil {
			logger.Fatal("failed to print migration status", zap.Error(err))
		}
	default:
		logger.Fatal("unsupported migration action", zap.String("action", *action))
	}
}

func resolveMigrationDir(flagDir string) (string, error) {
	candidates := make([]string, 0, 5)
	if strings.TrimSpace(flagDir) != "" {
		candidates = append(candidates, flagDir)
	}
	if envDir := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR")); envDir != "" {
		candidates = append(candidates, envDir)
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "migrations", "up"),
			filepath.Join(exeDir, "migrations", "up"),
		)
	}

	candidates = append(candidates, "migrations/up", "backend/migrations/up")

	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		info, statErr := os.Stat(cleaned)
		if statErr == nil && info.IsDir() {
			return cleaned, nil
		}
	}

	return "", fmt.Errorf("migration directory not found, checked: %s", strings.Join(candidates, ", "))
}

func ensureMigrationTable(db *gorm.DB) error {
	query := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    version VARCHAR(255) PRIMARY KEY,
    checksum VARCHAR(64) NOT NULL,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='Schema migration history';
`, migrationTableName)
	return db.Exec(query).Error
}

func applyMigrations(db *gorm.DB, dir string, logger *zap.Logger) error {
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}

	applied, err := loadAppliedMigrations(db)
	if err != nil {
		return err
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", file, err)
		}

		version := filepath.Base(file)
		checksum := fileChecksum(content)
		if appliedChecksum, ok := applied[version]; ok {
			if appliedChecksum != checksum {
				return fmt.Errorf("migration checksum mismatch for %s", version)
			}
			logger.Info("skip applied migration", zap.String("version", version))
			continue
		}

		statements := parseSQLStatements(string(content))
		logger.Info("applying migration",
			zap.String("version", version),
			zap.Int("statements", len(statements)),
		)

		for idx, statement := range statements {
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("failed to execute migration %s statement %d: %w", version, idx+1, err)
			}
		}

		if err := db.Exec(
			fmt.Sprintf("INSERT INTO %s (version, checksum) VALUES (?, ?)", migrationTableName),
			version,
			checksum,
		).Error; err != nil {
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}
	}

	return nil
}

func printMigrationStatus(db *gorm.DB, dir string) error {
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}

	applied, err := loadAppliedMigrations(db)
	if err != nil {
		return err
	}

	for _, file := range files {
		version := filepath.Base(file)
		status := "pending"
		if _, ok := applied[version]; ok {
			status = "applied"
		}
		fmt.Printf("%s\t%s\n", status, version)
	}

	return nil
}

func migrationFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, fmt.Errorf("failed to list migrations: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func loadAppliedMigrations(db *gorm.DB) (map[string]string, error) {
	var records []appliedMigration
	query := fmt.Sprintf("SELECT version, checksum FROM %s ORDER BY version ASC", migrationTableName)
	if err := db.Raw(query).Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}

	result := make(map[string]string, len(records))
	for _, record := range records {
		result[record.Version] = record.Checksum
	}
	return result, nil
}

func fileChecksum(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func parseSQLStatements(content string) []string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	var (
		statements     []string
		builder        strings.Builder
		inSingleQuote  bool
		inDoubleQuote  bool
		inBacktick     bool
		inLineComment  bool
		inBlockComment bool
	)

	runes := []rune(content)
	for i := 0; i < len(runes); i++ {
		current := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if inLineComment {
			builder.WriteRune(current)
			if current == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			builder.WriteRune(current)
			if current == '*' && next == '/' {
				builder.WriteRune(next)
				i++
				inBlockComment = false
			}
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if current == '-' && next == '-' {
				builder.WriteRune(current)
				builder.WriteRune(next)
				i++
				inLineComment = true
				continue
			}
			if current == '/' && next == '*' {
				builder.WriteRune(current)
				builder.WriteRune(next)
				i++
				inBlockComment = true
				continue
			}
		}

		switch current {
		case '\'':
			if !inDoubleQuote && !inBacktick {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote && !inBacktick {
				inDoubleQuote = !inDoubleQuote
			}
		case '`':
			if !inSingleQuote && !inDoubleQuote {
				inBacktick = !inBacktick
			}
		case ';':
			if !inSingleQuote && !inDoubleQuote && !inBacktick {
				statement := strings.TrimSpace(builder.String())
				if statement != "" {
					statements = append(statements, statement)
				}
				builder.Reset()
				continue
			}
		}

		builder.WriteRune(current)
	}

	last := strings.TrimSpace(builder.String())
	if last != "" {
		statements = append(statements, last)
	}

	return statements
}
