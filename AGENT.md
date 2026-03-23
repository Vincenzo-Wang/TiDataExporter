# AGENT.md - AI 开发规范文档

> 本文档供 AI Agent 使用，包含项目架构、开发规范、功能模块详情和迭代指南。

---

## 项目概览

### 项目名称
Claw Export Platform - 多租户 TiDB 数据导出平台

### 技术栈
- **后端**: Go 1.25+ / Gin / GORM / Redis / MySQL 8.0
- **前端**: React 18 / TypeScript / Vite / Ant Design 5 / Zustand
- **基础设施**: Docker / Docker Compose / Nginx

### 核心功能
1. 多租户数据导出（CSV/SQL/Parquet）
2. S3 兼容存储集成
3. 异步任务处理（Worker Pool）
4. 管理后台（React SPA）
5. API 开放接口

---

## 架构设计

### 系统架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                         客户端                                   │
│  ┌─────────────────┐              ┌─────────────────┐           │
│  │   管理后台 SPA   │              │   开放 API 客户端 │           │
│  │   (React)       │              │   (HTTP Client)  │           │
│  └────────┬────────┘              └────────┬────────┘           │
└───────────┼─────────────────────────────────┼───────────────────┘
            │ /api/v1/admin                  │ /api/v1/export
            │ JWT Auth                       │ API Key/Secret Auth
            ▼                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                      API Gateway (Gin)                          │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Middleware Chain                                         │   │
│  │  CORS → Logger → Recovery → RateLimit → Auth → RBAC     │   │
│  └──────────────────────────────────────────────────────────┘   │
│                            │                                    │
│  ┌──────────┬──────────┬───┴───┬──────────┬──────────┐         │
│  │  Auth    │  Task    │Tenant │  Config  │ Statistics│         │
│  │ Handlers │ Handlers │Handlr │ Handlers │ Handlers  │         │
│  └────┬─────┴────┬─────┴───┬───┴────┬─────┴─────┬─────┘         │
└───────┼──────────┼─────────┼────────┼───────────┼───────────────┘
        │          │         │        │           │
        ▼          ▼         ▼        ▼           ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Service Layer                                │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐   │
│  │ TaskService│ │ S3Service  │ │TaskManager │ │CleanService│   │
│  └─────┬──────┘ └─────┬──────┘ └─────┬──────┘ └─────┬──────┘   │
└────────┼──────────────┼──────────────┼──────────────┼───────────┘
         │              │              │              │
         ▼              ▼              ▼              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Infrastructure Layer                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │  MySQL   │  │  Redis   │  │ Dumpling │  │    S3    │        │
│  │ Database │  │  Cache   │  │  CLI     │  │ Storage  │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

### 数据流

```
用户请求 → API Gateway → 认证中间件 → 业务处理 → Worker 队列 → Dumpling 执行 → S3 上传 → 返回结果
```

---

## 目录结构详解

### 后端目录结构

```
backend/
├── api/                        # API 层
│   ├── middleware/             # 中间件
│   │   ├── auth.go            # JWT/API Key 认证
│   │   ├── auth_test.go       # 认证测试
│   │   ├── ratelimit.go       # 限流中间件
│   │   ├── rbac.go            # RBAC 权限控制
│   │   └── rbac_test.go       # RBAC 测试
│   ├── utils/                  # 工具函数
│   │   └── response.go        # 统一响应格式
│   ├── v1/                     # API v1 版本
│   │   └── handlers.go        # 所有 API handlers
│   └── router.go              # 路由配置
├── cmd/
│   └── server/
│       └── main.go            # 服务入口
├── config/
│   ├── config.go              # 配置加载
│   └── config.yaml            # 配置文件
├── migrations/                 # 数据库迁移
│   └── 001_init.sql           # 初始化脚本
├── models/                     # 数据模型
│   ├── admin.go               # 管理员模型
│   ├── tenant.go              # 租户模型
│   ├── task.go                # 任务模型
│   ├── tidb_config.go         # TiDB 配置模型
│   ├── s3_config.go           # S3 配置模型
│   └── audit_log.go           # 审计日志模型
├── pkg/                        # 公共包
│   ├── encryption/
│   │   └── encryption.go      # AES-256 加密
│   └── queue/
│       └── queue.go           # 任务队列
├── services/                   # 业务服务
│   ├── cleanup/
│   │   ├── cleanup.go         # 清理服务
│   │   └── cleanup_test.go    # 清理测试
│   ├── export/
│   │   └── executor.go        # 导出执行器
│   ├── s3/
│   │   └── s3.go              # S3 服务
│   └── task/
│       ├── manager.go         # 任务管理器
│       └── manager_test.go    # 任务管理测试
├── workers/
│   ├── worker.go              # Worker 实现
│   └── worker_test.go         # Worker 测试
├── go.mod
└── go.sum
```

### 前端目录结构

```
frontend/
├── src/
│   ├── components/             # 组件
│   │   └── Layout/
│   │       └── MainLayout.tsx # 主布局
│   ├── pages/                  # 页面
│   │   ├── Login.tsx          # 登录页
│   │   ├── Dashboard.tsx      # 仪表盘
│   │   ├── Tasks.tsx          # 任务列表
│   │   ├── TaskDetail.tsx     # 任务详情
│   │   ├── Tenants.tsx        # 租户管理
│   │   ├── TenantDetail.tsx   # 租户详情
│   │   ├── TiDBConfigs.tsx    # TiDB 配置
│   │   ├── S3Configs.tsx      # S3 配置
│   │   └── Statistics.tsx     # 统计报表
│   ├── services/
│   │   └── api.ts             # API 客户端
│   ├── stores/
│   │   └── auth.ts            # 认证状态
│   ├── styles/
│   │   └── global.css         # 全局样式
│   ├── types/
│   │   └── index.ts           # TypeScript 类型
│   ├── App.tsx                # 应用入口
│   └── main.tsx               # React 入口
├── nginx.conf                  # Nginx 配置
├── package.json
├── tsconfig.json
└── vite.config.ts
```

---

## 核心模块说明

### 1. 认证模块 (Authentication)

#### JWT 认证（管理后台）
```go
// 文件: api/middleware/auth.go

// JWTAuth JWT认证中间件
func JWTAuth(cfg *config.Config, db *gorm.DB, logger *zap.Logger) gin.HandlerFunc

// GenerateToken 生成JWT Token
func GenerateToken(cfg *config.Config, admin *models.Admin) (string, error)

// GetAdminFromContext 从上下文获取管理员信息
func GetAdminFromContext(c *gin.Context) (*models.Admin, bool)
```

#### API Key/Secret 认证（开放 API）
```go
// APIKeyAuth API Key认证中间件
func APIKeyAuth(db *gorm.DB, encryptor *encryption.Encryptor, logger *zap.Logger) gin.HandlerFunc

// 验证流程：
// 1. 从 Header 获取 X-API-Key 和 X-API-Secret
// 2. 根据 API Key 查询租户
// 3. 解密存储的 API Secret（AES-256）
// 4. 常量时间比较防止时序攻击
```

### 2. 任务模块 (Task Management)

#### 任务状态流转
```
pending(0) → running(1) → success(2)
                    ↓
                  failed(3) → retry → running(1)
                    ↓
                canceled(4)
                    ↓
                timeout(5)
```

#### 任务管理器
```go
// 文件: services/task/manager.go

type TaskManager struct {
    db         *gorm.DB
    encryptor  *encryption.Encryptor
    logger     *zap.Logger
    cancelFuncs map[int64]context.CancelFunc
}

// CancelTask 取消任务（可终止运行中的 Dumpling 进程）
func (m *TaskManager) CancelTask(taskID int64, reason string) error

// RetryTask 重试失败任务
func (m *TaskManager) RetryTask(taskID int64) error

// CheckTimeout 检查超时任务
func (m *TaskManager) CheckTimeout(ctx context.Context) error
```

### 3. Worker Pool

```go
// 文件: workers/worker.go

type Worker struct {
    id       int
    db       *gorm.DB
    executor *export.Executor
    logger   *zap.Logger
}

// Start 启动 Worker
func (w *Worker) Start(ctx context.Context, taskQueue <-chan int64)

// 处理流程：
// 1. 从队列获取任务 ID
// 2. 加载任务详情和配置
// 3. 执行 Dumpling 导出
// 4. 上传到 S3
// 5. 更新任务状态
```

### 4. 限流模块 (Rate Limiting)

```go
// 文件: api/middleware/ratelimit.go

// TokenBucketLimiter 令牌桶限流
type TokenBucketLimiter struct {
    rate  int           // 令牌生成速率
    burst int           // 桶容量
    store *redis.Client // Redis 存储
}

// SlidingWindowLimiter 滑动窗口限流
type SlidingWindowLimiter struct {
    windowSize time.Duration
    maxRequests int
    store      *redis.Client
}

// RateLimit 限流中间件
func RateLimit(limiter RateLimiter, keyFunc KeyFunc) gin.HandlerFunc
```

### 5. RBAC 权限控制

```go
// 文件: api/middleware/rbac.go

// 角色定义
const (
    RoleSuperAdmin = "super_admin"  // 超级管理员
    RoleAdmin      = "admin"        // 管理员
    RoleOperator   = "operator"     // 运营人员
    RoleViewer     = "viewer"       // 只读用户
)

// 权限定义
var permissions = map[string][]string{
    "task:create":   {RoleSuperAdmin, RoleAdmin},
    "task:cancel":   {RoleSuperAdmin, RoleAdmin, RoleOperator},
    "tenant:create": {RoleSuperAdmin},
    "tenant:update": {RoleSuperAdmin, RoleAdmin},
    // ...
}

// RequirePermission 权限检查中间件
func RequirePermission(permission string) gin.HandlerFunc
```

### 6. 加密模块

```go
// 文件: pkg/encryption/encryption.go

type Encryptor struct {
    key []byte // 32 字节密钥
}

// Encrypt AES-256-GCM 加密
func (e *Encryptor) Encrypt(plaintext string) (string, error)

// Decrypt AES-256-GCM 解密
func (e *Encryptor) Decrypt(ciphertext string) (string, error)

// 用途：
// - API Secret 加密存储
// - 数据库密码加密存储
// - S3 密钥加密存储
```

---

## 数据模型

### 核心表结构

#### tenants（租户表）
```sql
CREATE TABLE tenants (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL COMMENT '租户名称',
    code VARCHAR(50) NOT NULL UNIQUE COMMENT '租户编码',
    contact_email VARCHAR(255) NOT NULL COMMENT '联系邮箱',
    api_key VARCHAR(64) NOT NULL UNIQUE COMMENT 'API Key',
    api_secret_encrypted TEXT NOT NULL COMMENT '加密后的 API Secret',
    quota_daily INT NOT NULL DEFAULT 100 COMMENT '日配额',
    quota_used_today INT NOT NULL DEFAULT 0 COMMENT '今日已用',
    quota_monthly INT NOT NULL DEFAULT 3000 COMMENT '月配额',
    quota_used_month INT NOT NULL DEFAULT 0 COMMENT '本月已用',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '状态：1启用 0禁用',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_api_key (api_key),
    INDEX idx_status (status)
);
```

#### export_tasks（导出任务表）
```sql
CREATE TABLE export_tasks (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL COMMENT '租户ID',
    task_no VARCHAR(32) NOT NULL UNIQUE COMMENT '任务编号',
    tidb_config_name VARCHAR(100) NOT NULL COMMENT 'TiDB配置名称',
    s3_config_name VARCHAR(100) NOT NULL COMMENT 'S3配置名称',
    sql_text TEXT NOT NULL COMMENT 'SQL语句',
    filetype VARCHAR(20) NOT NULL DEFAULT 'csv' COMMENT '文件类型',
    compress VARCHAR(20) NOT NULL DEFAULT 'none' COMMENT '压缩方式',
    s3_path VARCHAR(500) COMMENT 'S3存储路径',
    file_size BIGINT DEFAULT 0 COMMENT '文件大小(字节)',
    row_count BIGINT DEFAULT 0 COMMENT '数据行数',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态',
    progress INT NOT NULL DEFAULT 0 COMMENT '进度(0-100)',
    error_message TEXT COMMENT '错误信息',
    retry_count INT NOT NULL DEFAULT 0 COMMENT '重试次数',
    priority INT NOT NULL DEFAULT 5 COMMENT '优先级(1-10)',
    retention_hours INT NOT NULL DEFAULT 168 COMMENT '保留时间(小时)',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME COMMENT '开始时间',
    completed_at DATETIME COMMENT '完成时间',
    expired_at DATETIME COMMENT '过期时间',
    canceled_at DATETIME COMMENT '取消时间',
    cancel_reason VARCHAR(255) COMMENT '取消原因',
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at),
    INDEX idx_task_no (task_no)
);
```

---

## API 端点参考

### 管理端 API

| 端点 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/api/v1/admin/auth/login` | POST | 无 | 管理员登录 |
| `/api/v1/admin/auth/refresh` | POST | JWT | 刷新 Token |
| `/api/v1/admin/auth/logout` | POST | JWT | 登出 |
| `/api/v1/admin/auth/profile` | GET | JWT | 获取当前用户信息 |
| `/api/v1/admin/tasks` | GET | JWT | 任务列表（分页） |
| `/api/v1/admin/tasks/:id` | GET | JWT | 任务详情 |
| `/api/v1/admin/tasks/:id/cancel` | POST | JWT | 取消任务 |
| `/api/v1/admin/tasks/:id/retry` | POST | JWT | 重试任务 |
| `/api/v1/admin/tenants` | GET | JWT | 租户列表 |
| `/api/v1/admin/tenants/:id` | GET | JWT | 租户详情 |
| `/api/v1/admin/tenants` | POST | JWT | 创建租户 |
| `/api/v1/admin/tenants/:id` | PUT | JWT | 更新租户 |
| `/api/v1/admin/tenants/:id` | DELETE | JWT | 删除租户 |
| `/api/v1/admin/tenants/:id/regenerate-keys` | POST | JWT | 重置密钥 |
| `/api/v1/admin/tidb-configs` | GET | JWT | TiDB 配置列表 |
| `/api/v1/admin/s3-configs` | GET | JWT | S3 配置列表 |
| `/api/v1/admin/statistics/overview` | GET | JWT | 统计概览 |
| `/api/v1/admin/statistics/daily` | GET | JWT | 每日统计 |

### 开放 API

| 端点 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/api/v1/export/tasks` | POST | API Key | 创建导出任务 |
| `/api/v1/export/tasks/:id` | GET | API Key | 查询任务状态 |
| `/api/v1/export/tasks/:id/cancel` | POST | API Key | 取消任务 |

---

## 开发规范

### Go 代码规范

#### 1. 错误处理
```go
// ✅ 正确：返回错误让上层处理
func (s *Service) DoSomething() error {
    if err := doWork(); err != nil {
        return fmt.Errorf("failed to do work: %w", err)
    }
    return nil
}

// ❌ 错误：吞掉错误
func (s *Service) DoSomething() {
    if err := doWork(); err != nil {
        log.Println(err) // 仅记录日志
    }
}
```

#### 2. Context 传递
```go
// ✅ 正确：context 作为第一个参数
func (s *Service) ProcessTask(ctx context.Context, taskID int64) error {
    // 使用 ctx 控制超时和取消
    return s.db.WithContext(ctx).First(&task, taskID).Error
}
```

#### 3. 日志规范
```go
// ✅ 正确：结构化日志
logger.Info("task started",
    zap.Int64("task_id", taskID),
    zap.String("tenant", tenantName),
)

// ❌ 错误：字符串拼接
logger.Info(fmt.Sprintf("task %d started for tenant %s", taskID, tenantName))
```

### React/TypeScript 代码规范

#### 1. 组件定义
```tsx
// ✅ 正确：函数组件 + 类型定义
interface TaskListProps {
  tenantId?: number;
  onTaskSelect?: (task: ExportTask) => void;
}

export default function TaskList({ tenantId, onTaskSelect }: TaskListProps) {
  // ...
}
```

#### 2. API 调用
```tsx
// ✅ 正确：使用 api service + 错误处理
const fetchData = async () => {
  try {
    const response = await api.get<ApiResponse<Task[]>>('/admin/tasks');
    if (response.data.code === 0) {
      setTasks(response.data.data);
    } else {
      message.error(response.data.message);
    }
  } catch (error) {
    console.error('Failed to fetch tasks:', error);
    message.error('获取任务列表失败');
  }
};
```

#### 3. 状态管理
```tsx
// 使用 Zustand
interface AuthState {
  token: string | null;
  user: Admin | null;
  login: (token: string, user: Admin) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      login: (token, user) => set({ token, user, isAuthenticated: true }),
      logout: () => set({ token: null, user: null, isAuthenticated: false }),
    }),
    { name: 'auth-storage' }
  )
);
```

---

## 功能迭代指南

### 添加新的 API 端点

> ⚠️ **重要规范**: 每次新增或修改 API 接口时，必须同步更新 `docs/API.md` 接口文档！

1. **定义路由** (`api/router.go`)
```go
v1Group.GET("/new-endpoint", handler.NewFunction)
```

2. **创建 Handler** (`api/v1/handlers.go`)
```go
func (h *Handler) NewFunction(c *gin.Context) {
    // 1. 参数验证
    // 2. 业务逻辑
    // 3. 返回响应
}
```

3. **添加中间件** (如需要)
```go
adminGroup.GET("/protected", middleware.RequirePermission("resource:action"), handler.ProtectedFunction)
```

4. **更新 API 文档** (`docs/API.md`)
   - 添加接口说明
   - 添加请求/响应示例
   - 添加错误码说明

### API 文档维护规范

#### 文档位置
- **接口文档**: `docs/API.md`
- **开发文档**: `docs/DEVELOPMENT.md`

#### 必须更新文档的场景
1. 新增 API 端点
2. 修改 API 参数（新增、删除、修改字段）
3. 修改响应格式
4. 新增错误码
5. 修改认证方式

#### 文档格式要求

```markdown
### N. 接口名称

简要描述接口功能。

**请求**

\`\`\`http
METHOD /api/v1/path
Header: Value
\`\`\`

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int | 资源 ID |

**请求体**

\`\`\`json
{
  "field": "value"
}
\`\`\`

**参数说明**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| field | string | 是 | 字段说明 |

**响应**

\`\`\`json
{
  "code": 0,
  "message": "success",
  "data": { ... }
}
\`\`\`

**错误情况**

| HTTP 状态码 | 错误码 | 说明 |
|-------------|--------|------|
| 400 | 40001 | 参数错误 |
```

#### 检查清单

在提交 API 相关代码前，请确认：

- [ ] `docs/API.md` 已更新
- [ ] 新增接口已添加完整文档
- [ ] 修改的接口已更新文档
- [ ] 新增错误码已添加到错误码表
- [ ] 请求/响应示例格式正确

### 添加新的前端页面

1. **创建页面组件** (`src/pages/NewPage.tsx`)
2. **添加路由** (`src/App.tsx`)
```tsx
<Route path="new-page" element={<NewPage />} />
```
3. **添加菜单项** (`src/components/Layout/MainLayout.tsx`)
```tsx
{
  key: '/new-page',
  icon: <IconComponent />,
  label: '新页面',
}
```

### 添加新的数据模型

1. **创建模型** (`models/new_model.go`)
```go
type NewModel struct {
    gorm.Model
    Field1 string `gorm:"type:varchar(100);not null"`
    // ...
}
```

2. **创建迁移脚本** (`migrations/002_add_new_table.sql`)
3. **更新配置加载** (如需要)

---

## 测试规范

### 后端测试

```go
// 文件: services/task/manager_test.go

func TestTaskManager_CancelTask(t *testing.T) {
    // 1. 准备测试数据
    db := setupTestDB(t)
    defer db.Close()
    
    // 2. 创建测试实例
    manager := NewTaskManager(db, nil, zap.NewNop())
    
    // 3. 执行测试
    err := manager.CancelTask(1, "test")
    
    // 4. 验证结果
    assert.NoError(t, err)
}
```

### 前端测试

```tsx
// 文件: components/__tests__/TaskList.test.tsx

import { render, screen } from '@testing-library/react';
import TaskList from '../TaskList';

test('renders task list', () => {
  render(<TaskList />);
  expect(screen.getByText('任务列表')).toBeInTheDocument();
});
```

---

## 常见问题

### Q: 如何调试后端服务？
```bash
# 启用 debug 日志
LOG_LEVEL=debug go run cmd/server/main.go

# 或在 docker 中
docker-compose exec backend sh
cat /app/logs/app.log
```

### Q: 如何添加新的导出格式？
1. 修改 `services/export/executor.go` 的 `Execute` 方法
2. 添加新的格式处理逻辑
3. 更新前端 `Tasks.tsx` 的 `fileTypeMap`

### Q: 如何修改配额限制？
1. 修改 `models/tenant.go` 的默认值
2. 或通过管理后台修改单个租户配额

---

## 更新日志

### v1.0.0 (2026-03-23)
- 初始版本发布
- 完成核心功能：多租户、导出、S3 上传
- 完成管理后台：登录、任务管理、配置管理
- 完成部署配置：Docker Compose、一键部署脚本
- 完成 API 接口文档 (`docs/API.md`)
- 完成数据库自动迁移

---

## 待办事项 (Roadmap)

- [ ] WebSocket 实时任务状态推送
- [ ] 支持更多导出格式（JSON、Excel）
- [ ] 数据脱敏功能
- [ ] 自定义 SQL 模板
- [ ] Prometheus 监控指标
- [ ] 多语言支持（i18n）
- [ ] Swagger/OpenAPI 集成

---

*最后更新: 2026-03-23*
