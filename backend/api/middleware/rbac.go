package middleware

import (
	"claw-export-platform/api/utils"
	"claw-export-platform/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RBACConfig RBAC 配置
type RBACConfig struct {
	// 角色权限映射
	RolePermissions map[string][]string
}

// DefaultRBACConfig 默认 RBAC 配置
var DefaultRBACConfig = RBACConfig{
	RolePermissions: map[string][]string{
		"admin": {
			"tenant:create", "tenant:read", "tenant:update", "tenant:delete",
			"task:create", "task:read", "task:update", "task:delete", "task:cancel",
			"config:create", "config:read", "config:update", "config:delete",
			"template:create", "template:read", "template:update", "template:delete",
			"audit:read",
			"statistics:read",
		},
		"operator": {
			"tenant:read",
			"task:create", "task:read", "task:cancel",
			"config:create", "config:read", "config:update",
			"template:create", "template:read", "template:update",
			"audit:read",
			"statistics:read",
		},
	},
}

// RBAC RBAC 中间件
type RBAC struct {
	config RBACConfig
	logger *zap.Logger
}

// NewRBAC 创建 RBAC 中间件
func NewRBAC(config RBACConfig, logger *zap.Logger) *RBAC {
	return &RBAC{
		config: config,
		logger: logger,
	}
}

// RequirePermission 检查权限中间件
func (r *RBAC) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, exists := c.Get(string(ContextKeyAdmin))
		if !exists {
			utils.Unauthorized(c, "未登录")
			c.Abort()
			return
		}

		adminModel, ok := admin.(*models.Admin)
		if !ok {
			utils.InternalError(c, "用户信息异常")
			c.Abort()
			return
		}

		if !r.hasPermission(adminModel.Role, permission) {
			r.logger.Warn("permission denied",
				zap.Int64("admin_id", adminModel.ID),
				zap.String("role", adminModel.Role),
				zap.String("permission", permission),
			)
			utils.Forbidden(c, "权限不足")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireRole 检查角色中间件
func (r *RBAC) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, exists := c.Get(string(ContextKeyAdmin))
		if !exists {
			utils.Unauthorized(c, "未登录")
			c.Abort()
			return
		}

		adminModel, ok := admin.(*models.Admin)
		if !ok {
			utils.InternalError(c, "用户信息异常")
			c.Abort()
			return
		}

		hasRole := false
		for _, role := range roles {
			if adminModel.Role == role {
				hasRole = true
				break
			}
		}

		if !hasRole {
			r.logger.Warn("role denied",
				zap.Int64("admin_id", adminModel.ID),
				zap.String("role", adminModel.Role),
				zap.Strings("required_roles", roles),
			)
			utils.Forbidden(c, "角色权限不足")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAdmin 只允许管理员角色
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, exists := c.Get(string(ContextKeyAdmin))
		if !exists {
			utils.Unauthorized(c, "未登录")
			c.Abort()
			return
		}

		adminModel, ok := admin.(*models.Admin)
		if !ok {
			utils.InternalError(c, "用户信息异常")
			c.Abort()
			return
		}

		if adminModel.Role != "admin" {
			utils.Forbidden(c, "需要管理员权限")
			c.Abort()
			return
		}

		c.Next()
	}
}

// hasPermission 检查角色是否拥有指定权限
func (r *RBAC) hasPermission(role, permission string) bool {
	permissions, exists := r.config.RolePermissions[role]
	if !exists {
		return false
	}

	for _, p := range permissions {
		if p == permission {
			return true
		}
	}

	return false
}

// GetAdminFromContext 从上下文获取管理员信息
func GetAdminFromContext(c *gin.Context) (*models.Admin, bool) {
	admin, exists := c.Get(string(ContextKeyAdmin))
	if !exists {
		return nil, false
	}

	adminModel, ok := admin.(*models.Admin)
	return adminModel, ok
}

// GetTenantFromContext 从上下文获取租户信息
func GetTenantFromContext(c *gin.Context) (*models.Tenant, bool) {
	tenant, exists := c.Get(string(ContextKeyTenant))
	if !exists {
		return nil, false
	}

	tenantModel, ok := tenant.(*models.Tenant)
	return tenantModel, ok
}

// GetTenantIDFromContext 从上下文获取租户ID
func GetTenantIDFromContext(c *gin.Context) (int64, bool) {
	tenantID, exists := c.Get(string(ContextKeyTenantID))
	if !exists {
		return 0, false
	}

	id, ok := tenantID.(int64)
	return id, ok
}

// GetAdminIDFromContext 从上下文获取管理员ID
func GetAdminIDFromContext(c *gin.Context) (int64, bool) {
	adminID, exists := c.Get(string(ContextKeyAdminID))
	if !exists {
		return 0, false
	}

	id, ok := adminID.(int64)
	return id, ok
}
