# Claw Export Platform

一个多租户 TiDB 数据导出平台，支持将数据导出到 S3 兼容存储。

## 功能特性

- 🏢 **多租户架构** - 完整的租户隔离和配额管理
- 📤 **灵活导出** - 支持 CSV、SQL、Parquet 格式
- ☁️ **S3 集成** - 直接上传到各种 S3 兼容存储
- ⚡ **高性能** - 基于 Dumpling 的 TiDB 数据导出
- 🔐 **安全认证** - JWT + API Key/Secret 双重认证
- 📊 **管理后台** - 现代化的 React 管理界面
- 🔄 **任务管理** - 异步任务处理、自动重试、超时控制
- 📈 **监控统计** - 内置统计报表和审计日志

## 快速开始

### 环境要求

- Docker 20.10+
- Docker Compose 2.0+
- （可选）Node.js 18+ 用于前端开发
- （可选）Go 1.22+ 用于后端开发

### 一键部署

```bash
# 1. 克隆项目
git clone <repository-url>
cd claw-export-platform

# 2. 复制环境变量配置
cp .env.example .env

# 3. 修改配置（重要！修改默认密码）
vim .env

# 4. 一键启动
./deploy.sh

# 5. 访问应用
# 前端：http://localhost
# 后端：http://localhost:8080
# 默认管理员：admin / admin123（首次登录后请立即修改！）
```

### 开发模式

#### 后端开发

```bash
cd backend

# 安装依赖
go mod download

# 复制配置文件
cp config/config.yaml.example config/config.yaml

# 启动服务
go run cmd/server/main.go
```

#### 前端开发

```bash
cd frontend

# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 访问 http://localhost:3000
```

## 项目结构

```
claw-export-platform/
├── backend/                    # Go 后端服务
│   ├── api/                    # API 层
│   │   ├── middleware/         # 中间件（认证、限流、RBAC）
│   │   ├── utils/              # 工具函数
│   │   ├── v1/                 # v1 版本 API
│   │   └── router.go           # 路由配置
│   ├── cmd/server/             # 服务入口
│   ├── config/                 # 配置文件
│   ├── migrations/             # 数据库迁移脚本
│   ├── models/                 # 数据模型
│   ├── pkg/                    # 公共包
│   │   ├── encryption/         # AES 加密工具
│   │   └── queue/              # 任务队列
│   ├── services/               # 业务服务
│   │   ├── cleanup/            # 清理服务
│   │   ├── export/             # 导出执行器
│   │   ├── s3/                 # S3 服务
│   │   └── task/               # 任务管理
│   └── workers/                # Worker Pool
├── frontend/                   # React 前端
│   ├── src/
│   │   ├── components/         # 组件
│   │   ├── pages/              # 页面
│   │   ├── services/           # API 服务
│   │   ├── stores/             # 状态管理
│   │   └── types/              # TypeScript 类型
│   └── nginx.conf              # Nginx 配置
├── docs/                       # 文档
│   ├── API.md                  # API 接口文档
│   └── DEVELOPMENT.md          # 开发部署文档
├── AGENT.md                    # AI Agent 开发规范
├── deploy.sh                   # 一键部署脚本
├── docker-compose.yml          # Docker 编排
├── .env.example                # 环境变量模板
└── README.md
```

## API 概览

> 📖 **详细 API 文档**: 请参阅 [docs/API.md](docs/API.md)

### 管理端 API（JWT 认证）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/admin/auth/login` | POST | 管理员登录 |
| `/api/v1/admin/auth/refresh` | POST | 刷新 Token |
| `/api/v1/admin/tasks` | GET | 任务列表 |
| `/api/v1/admin/tenants` | GET | 租户列表 |
| `/api/v1/admin/tidb-configs` | GET | TiDB 配置列表 |
| `/api/v1/admin/s3-configs` | GET | S3 配置列表 |
| `/api/v1/admin/statistics/overview` | GET | 统计概览 |

### 开放 API（API Key/Secret 认证）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/export/tasks` | POST | 创建导出任务 |
| `/api/v1/export/tasks/:id` | GET | 查询任务状态 |
| `/api/v1/export/tasks/:id/cancel` | POST | 取消任务 |

## 使用示例

### 创建导出任务

```bash
curl -X POST http://localhost:8080/api/v1/export/tasks \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -H "X-API-Secret: your-api-secret" \
  -d '{
    "tidb_config_name": "生产环境",
    "sql_text": "SELECT * FROM orders WHERE created_at >= '\''2025-01-01'\''",
    "filetype": "csv",
    "compress": "gzip",
    "retention_hours": 168
  }'
```

### 查询任务状态

```bash
curl -X GET "http://localhost:8080/api/v1/export/tasks/12345" \
  -H "X-API-Key: your-api-key" \
  -H "X-API-Secret: your-api-secret"
```

## 安全特性

- **数据加密** - 敏感数据使用 AES-256-GCM 加密存储
- **双重认证** - JWT Token + API Key/Secret 机制
- **防护措施** - SQL 注入防护、时序攻击防护
- **访问限制** - 令牌桶 + 滑动窗口限流算法
- **审计日志** - 完整的操作日志记录

## 技术栈

### 后端
- Go 1.22+ / Gin Framework
- MySQL 8.0
- Redis 7
- Dumpling (TiDB 数据导出工具)

### 前端
- React 18 + TypeScript
- Vite 构建工具
- Ant Design 5 组件库
- Zustand 状态管理

## 许可证

MIT License
