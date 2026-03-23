package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"claw-export-platform/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRBAC_HasPermission(t *testing.T) {
	rbac := NewRBAC(DefaultRBACConfig, zap.NewNop())

	tests := []struct {
		name       string
		role       string
		permission string
		want       bool
	}{
		{"admin has tenant:create", "admin", "tenant:create", true},
		{"admin has tenant:delete", "admin", "tenant:delete", true},
		{"operator has tenant:read", "operator", "tenant:read", true},
		{"operator missing tenant:delete", "operator", "tenant:delete", false},
		{"unknown role", "unknown", "tenant:read", false},
		{"admin has task:cancel", "admin", "task:cancel", true},
		{"operator has task:cancel", "operator", "task:cancel", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rbac.hasPermission(tt.role, tt.permission); got != tt.want {
				t.Errorf("hasPermission(%s, %s) = %v, want %v", tt.role, tt.permission, got, tt.want)
			}
		})
	}
}

func TestRBAC_RequirePermission(t *testing.T) {
	rbac := NewRBAC(DefaultRBACConfig, zap.NewNop())

	tests := []struct {
		name       string
		admin      *models.Admin
		permission string
		wantStatus int
	}{
		{
			name: "admin with valid permission",
			admin: &models.Admin{
				ID:       1,
				Username: "admin",
				Role:     "admin",
				Status:   1,
			},
			permission: "tenant:delete",
			wantStatus: http.StatusOK,
		},
		{
			name: "operator without delete permission",
			admin: &models.Admin{
				ID:       2,
				Username: "operator",
				Role:     "operator",
				Status:   1,
			},
			permission: "tenant:delete",
			wantStatus: http.StatusForbidden,
		},
		{
			name: "operator with valid permission",
			admin: &models.Admin{
				ID:       2,
				Username: "operator",
				Role:     "operator",
				Status:   1,
			},
			permission: "task:read",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyAdmin), tt.admin)
				c.Set(string(ContextKeyAdminID), tt.admin.ID)
				c.Next()
			})
			router.Use(rbac.RequirePermission(tt.permission))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "success"})
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestRBAC_RequireRole(t *testing.T) {
	rbac := NewRBAC(DefaultRBACConfig, zap.NewNop())

	tests := []struct {
		name       string
		admin      *models.Admin
		roles      []string
		wantStatus int
	}{
		{
			name: "admin with admin role required",
			admin: &models.Admin{
				ID:       1,
				Username: "admin",
				Role:     "admin",
				Status:   1,
			},
			roles:      []string{"admin"},
			wantStatus: http.StatusOK,
		},
		{
			name: "operator with admin role required",
			admin: &models.Admin{
				ID:       2,
				Username: "operator",
				Role:     "operator",
				Status:   1,
			},
			roles:      []string{"admin"},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "operator with multiple roles required",
			admin: &models.Admin{
				ID:       2,
				Username: "operator",
				Role:     "operator",
				Status:   1,
			},
			roles:      []string{"admin", "operator"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyAdmin), tt.admin)
				c.Set(string(ContextKeyAdminID), tt.admin.ID)
				c.Next()
			})
			router.Use(rbac.RequireRole(tt.roles...))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "success"})
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestRequireAdmin(t *testing.T) {
	tests := []struct {
		name       string
		admin      *models.Admin
		wantStatus int
	}{
		{
			name: "admin role allowed",
			admin: &models.Admin{
				ID:       1,
				Username: "admin",
				Role:     "admin",
				Status:   1,
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "operator role denied",
			admin: &models.Admin{
				ID:       2,
				Username: "operator",
				Role:     "operator",
				Status:   1,
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.Use(func(c *gin.Context) {
				if tt.admin != nil {
					c.Set(string(ContextKeyAdmin), tt.admin)
					c.Set(string(ContextKeyAdminID), tt.admin.ID)
				}
				c.Next()
			})
			router.Use(RequireAdmin())
			router.GET("/test", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "success"})
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestGetAdminFromContext(t *testing.T) {
	admin := &models.Admin{
		ID:       1,
		Username: "testadmin",
		Role:     "admin",
	}

	t.Run("admin exists in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(string(ContextKeyAdmin), admin)

		got, ok := GetAdminFromContext(c)
		if !ok {
			t.Error("expected ok to be true")
		}
		if got.ID != admin.ID {
			t.Errorf("got ID %d, want %d", got.ID, admin.ID)
		}
	})

	t.Run("admin not in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)

		_, ok := GetAdminFromContext(c)
		if ok {
			t.Error("expected ok to be false")
		}
	})
}

func TestGetTenantFromContext(t *testing.T) {
	tenant := &models.Tenant{
		ID:     1,
		Name:   "Test Tenant",
		Status: 1,
	}

	t.Run("tenant exists in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(string(ContextKeyTenant), tenant)

		got, ok := GetTenantFromContext(c)
		if !ok {
			t.Error("expected ok to be true")
		}
		if got.ID != tenant.ID {
			t.Errorf("got ID %d, want %d", got.ID, tenant.ID)
		}
	})

	t.Run("tenant not in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)

		_, ok := GetTenantFromContext(c)
		if ok {
			t.Error("expected ok to be false")
		}
	})
}

func TestGetTenantIDFromContext(t *testing.T) {
	t.Run("tenant_id exists in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(string(ContextKeyTenantID), int64(1))

		got, ok := GetTenantIDFromContext(c)
		if !ok {
			t.Error("expected ok to be true")
		}
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("tenant_id not in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)

		_, ok := GetTenantIDFromContext(c)
		if ok {
			t.Error("expected ok to be false")
		}
	})
}

func TestGetAdminIDFromContext(t *testing.T) {
	t.Run("admin_id exists in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set(string(ContextKeyAdminID), int64(1))

		got, ok := GetAdminIDFromContext(c)
		if !ok {
			t.Error("expected ok to be true")
		}
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("admin_id not in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)

		_, ok := GetAdminIDFromContext(c)
		if ok {
			t.Error("expected ok to be false")
		}
	})
}
