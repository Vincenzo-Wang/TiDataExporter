package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"claw-export-platform/api/utils"
	"claw-export-platform/config"
	"claw-export-platform/models"
	"claw-export-platform/pkg/encryption"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(&models.Tenant{}, &models.Admin{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// setupTestEncryptor 创建测试加密器
func setupTestEncryptor(t *testing.T) *encryption.Encryptor {
	enc, err := encryption.NewEncryptor("12345678901234567890123456789012") // 32 bytes for AES-256
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}
	return enc
}

// setupTestRouter 创建测试路由
func setupTestRouter() *gin.Engine {
	return gin.New()
}

func TestConstantTimeEqual(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"equal strings", "hello", "hello", true},
		{"different strings", "hello", "world", false},
		{"different lengths", "hello", "hell", false},
		{"empty strings", "", "", true},
		{"one empty", "hello", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := constantTimeEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("constantTimeEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIKeyAuth_MissingCredentials(t *testing.T) {
	db := setupTestDB(t)
	enc := setupTestEncryptor(t)
	logger := zap.NewNop()

	router := setupTestRouter()
	router.Use(APIKeyAuth(db, enc, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "success"})
	})

	// 测试缺少 API Key
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Secret", "secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// 测试缺少 API Secret
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "test-key")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAPIKeyAuth_InvalidAPIKey(t *testing.T) {
	db := setupTestDB(t)
	enc := setupTestEncryptor(t)
	logger := zap.NewNop()

	// 创建一个租户
	secretEncrypted, _ := enc.Encrypt("test-secret")
	tenant := &models.Tenant{
		Name:              "Test Tenant",
		APIKey:            "valid-key",
		APISecretEncrypted: secretEncrypted,
		Status:            1,
	}
	db.Create(tenant)

	router := setupTestRouter()
	router.Use(APIKeyAuth(db, enc, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "success"})
	})

	// 测试无效的 API Key
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	req.Header.Set("X-API-Secret", "test-secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAPIKeyAuth_InvalidAPISecret(t *testing.T) {
	db := setupTestDB(t)
	enc := setupTestEncryptor(t)
	logger := zap.NewNop()

	// 创建一个租户
	secretEncrypted, _ := enc.Encrypt("correct-secret")
	tenant := &models.Tenant{
		Name:              "Test Tenant",
		APIKey:            "valid-key",
		APISecretEncrypted: secretEncrypted,
		Status:            1,
	}
	db.Create(tenant)

	router := setupTestRouter()
	router.Use(APIKeyAuth(db, enc, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "success"})
	})

	// 测试无效的 API Secret
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "valid-key")
	req.Header.Set("X-API-Secret", "wrong-secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAPIKeyAuth_DisabledTenant(t *testing.T) {
	db := setupTestDB(t)
	enc := setupTestEncryptor(t)
	logger := zap.NewNop()

	// 创建一个禁用的租户
	secretEncrypted, _ := enc.Encrypt("test-secret")
	tenant := &models.Tenant{
		Name:              "Disabled Tenant",
		APIKey:            "disabled-key",
		APISecretEncrypted: secretEncrypted,
		Status:            0, // 禁用状态
	}
	db.Create(tenant)

	router := setupTestRouter()
	router.Use(APIKeyAuth(db, enc, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "disabled-key")
	req.Header.Set("X-API-Secret", "test-secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestAPIKeyAuth_Success(t *testing.T) {
	db := setupTestDB(t)
	enc := setupTestEncryptor(t)
	logger := zap.NewNop()

	// 创建一个有效的租户
	secretEncrypted, _ := enc.Encrypt("test-secret")
	tenant := &models.Tenant{
		Name:              "Valid Tenant",
		APIKey:            "valid-key",
		APISecretEncrypted: secretEncrypted,
		Status:            1,
	}
	db.Create(tenant)

	router := setupTestRouter()
	router.Use(APIKeyAuth(db, enc, logger))
	router.GET("/test", func(c *gin.Context) {
		tenantID, exists := c.Get(string(ContextKeyTenantID))
		if !exists {
			c.JSON(500, gin.H{"error": "tenant_id not found"})
			return
		}
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "valid-key")
	req.Header.Set("X-API-Secret", "test-secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestJWTAuth_MissingToken(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret:       "test-secret-key-for-jwt",
			TokenExpireHour: 24,
		},
	}

	router := setupTestRouter()
	router.Use(JWTAuth(cfg, db, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret:       "test-secret-key-for-jwt",
			TokenExpireHour: 24,
		},
	}

	router := setupTestRouter()
	router.Use(JWTAuth(cfg, db, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestJWTAuth_DisabledAdmin(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret:       "test-secret-key-for-jwt",
			TokenExpireHour: 24,
		},
	}

	// 创建一个禁用的管理员
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	admin := &models.Admin{
		Username:     "disabledadmin",
		PasswordHash: string(passwordHash),
		Role:         "admin",
		Status:       0, // 禁用状态
	}
	db.Create(admin)

	// 生成 Token
	token, _ := GenerateToken(cfg, admin)

	router := setupTestRouter()
	router.Use(JWTAuth(cfg, db, logger))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestJWTAuth_Success(t *testing.T) {
	db := setupTestDB(t)
	logger := zap.NewNop()
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret:       "test-secret-key-for-jwt",
			TokenExpireHour: 24,
		},
	}

	// 创建一个有效的管理员
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	admin := &models.Admin{
		Username:     "testadmin",
		PasswordHash: string(passwordHash),
		Role:         "admin",
		Status:       1,
	}
	db.Create(admin)

	// 生成 Token
	token, _ := GenerateToken(cfg, admin)

	router := setupTestRouter()
	router.Use(JWTAuth(cfg, db, logger))
	router.GET("/test", func(c *gin.Context) {
		adminID, exists := c.Get(string(ContextKeyAdminID))
		if !exists {
			c.JSON(500, gin.H{"error": "admin_id not found"})
			return
		}
		c.JSON(200, gin.H{"admin_id": adminID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGenerateToken(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			JWTSecret:       "test-secret-key-for-jwt",
			TokenExpireHour: 24,
		},
	}

	admin := &models.Admin{
		ID:       1,
		Username: "testadmin",
		Role:     "admin",
	}

	token, err := GenerateToken(cfg, admin)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if token == "" {
		t.Error("GenerateToken() returned empty token")
	}

	// 验证生成的 Token
	claims := &JWTClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(cfg.Security.JWTSecret), nil
	})

	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	if !parsedToken.Valid {
		t.Error("token is not valid")
	}

	if claims.AdminID != admin.ID {
		t.Errorf("claims.AdminID = %d, want %d", claims.AdminID, admin.ID)
	}

	if claims.Username != admin.Username {
		t.Errorf("claims.Username = %s, want %s", claims.Username, admin.Username)
	}
}

func TestRequestID(t *testing.T) {
	router := setupTestRouter()
	router.Use(RequestID())
	router.GET("/test", func(c *gin.Context) {
		requestID, exists := c.Get(string(ContextKeyRequestID))
		if !exists {
			c.JSON(500, gin.H{"error": "request_id not found"})
			return
		}
		c.JSON(200, gin.H{"request_id": requestID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// 检查响应头中是否包含 Request ID
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header is empty")
	}
}

func TestRequestID_WithHeader(t *testing.T) {
	router := setupTestRouter()
	router.Use(RequestID())
	router.GET("/test", func(c *gin.Context) {
		requestID, _ := c.Get(string(ContextKeyRequestID))
		c.JSON(200, gin.H{"request_id": requestID})
	})

	existingID := "existing-request-id"
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", existingID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// 检查是否使用了传入的 Request ID
	if w.Header().Get("X-Request-ID") != existingID {
		t.Errorf("X-Request-ID header = %s, want %s", w.Header().Get("X-Request-ID"), existingID)
	}
}
