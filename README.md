# Claw Export Platform

一个多租户 TiDB 数据导出平台，支持将数据导出到 S3 兼容存储。

## 功能特性

- 🏢 **多租户架构** - 完整的租户隔离和配额管理
- 📤 **灵活导出** - 支持 CSV、SQL 格式，支持 Gzip/Snappy/Zstd 压缩
- ☁️ **S3 集成** - 直接上传到各种 S3 兼容存储
- ⚡ **高性能** - 基于 Dumpling 的 TiDB 数据导出
- 🔐 **安全认证** - JWT + API Key/Secret 双重认证
- 📊 **管理后台** - 现代化的 React 管理界面
- 🔄 **异步任务** - 基于 Redis 的任务队列，支持优先级和自动重试
- 📈 **监控统计** - 内置统计报表和审计日志

## 快速开始

### 环境要求

- Docker 20.10+
- Docker Compose 1.18.0+（兼容旧版本）
- Dumpling（通过 TiUP 安装）

### 安装 Dumpling

```bash
# 使用 TiUP 安装
tiup install dumpling

# 确认安装路径
tiup list --installed | grep dumpling
# 输出示例：dumpling    v8.5.5    /home/work/.tiup/components/dumpling/v8.5.5
```

### 一键部署

```bash
# 1. 克隆项目
git clone <repository-url>
cd claw-export-platform

# 2. 复制环境变量配置
cp .env.example .env

# 3. 修改配置（重要！）
vim .env
# 必须修改：
# - HOST_DUMPLING_PATH（Dumpling 路径）
# - JWT_SECRET（JWT 密钥）
# - AES_KEY（加密密钥，必须 32 字节）

# 4. 一键启动
chmod +x deploy.sh
./deploy.sh start

# 5. 访问应用
# 前端：http://localhost
# 后端：http://localhost:8080
# 默认管理员：admin / admin123（首次登录后请立即修改！）
```

### 部署脚本命令

```bash
./deploy.sh start       # 启动所有服务
./deploy.sh stop        # 停止所有服务
./deploy.sh restart     # 重启所有服务
./deploy.sh status      # 查看服务状态
./deploy.sh logs        # 查看日志
./deploy.sh migrate     # 执行数据库迁移
./deploy.sh backup      # 备份数据库
./deploy.sh help        # 显示帮助信息
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
│   │   ├── middleware/         # 中间件（认证、限流、CORS）
│   │   ├── v1/                 # v1 版本 API handlers
│   │   └── router.go           # 路由配置
│   ├── cmd/
│   │   ├── server/             # API 服务入口
│   │   └── worker/             # Worker 入口
│   ├── config/                 # 配置文件
│   ├── migrations/             # 数据库迁移脚本
│   │   ├── init.sh             # 自动初始化脚本
│   │   ├── up/                 # 升级迁移
│   │   └── down/               # 回滚迁移
│   ├── models/                 # 数据模型
│   ├── pkg/                    # 公共包
│   │   ├── database/           # 数据库连接
│   │   ├── encryption/         # AES 加密工具
│   │   └── queue/              # Redis 任务队列
│   ├── services/               # 业务服务
│   │   ├── cleanup/            # 过期文件清理
│   │   ├── export/             # Dumpling 执行器
│   │   ├── s3/                 # S3 上传服务
│   │   └── task/               # 任务管理
│   ├── workers/                # Worker Pool
│   ├── Dockerfile              # Backend 镜像
│   └── Dockerfile.worker       # Worker 镜像
├── frontend/                   # React 前端
│   ├── src/
│   │   ├── components/         # 组件
│   │   ├── pages/              # 页面
│   │   ├── services/           # API 服务
│   │   ├── stores/             # Zustand 状态管理
│   │   └── types/              # TypeScript 类型
│   ├── nginx.conf              # Nginx 配置
│   └── Dockerfile              # Frontend 镜像
├── docs/                       # 文档
│   ├── API.md                  # API 接口文档
│   ├── api-usage-guide.md      # API 使用指南
│   └── DEVELOPMENT.md          # 开发部署文档
├── AGENT.md                    # AI Agent 开发规范
├── deploy.sh                   # 一键部署脚本
├── docker-compose.yml          # Docker 编排
├── .env.example                # 环境变量模板
└── README.md
```

## 服务架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        用户请求                                  │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  claw-frontend (Nginx)                                          │
│  - 静态文件服务                                                  │
│  - /api 反向代理到 backend                                       │
│  - Port: 80                                                     │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  claw-backend (Go API)                                          │
│  - JWT 认证 / API Key 认证                                       │
│  - 租户管理、任务管理、配置管理                                    │
│  - 任务入队（Redis）                                             │
│  - Port: 8080                                                   │
└────────┬───────────────────────┬─────────────────────────────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐    ┌─────────────────────────────────────────┐
│  claw-mysql     │    │  claw-redis                             │
│  - 业务数据存储   │    │  - 任务队列（优先级队列）                 │
│  - Port: 3306   │    │  - 缓存                                  │
└─────────────────┘    │  - Port: 6379                          │
                       └───────────────────┬─────────────────────┘
                                           │
                                           ▼
                       ┌─────────────────────────────────────────┐
                       │  claw-worker (Go Worker)                │
                       │  - 从队列获取任务                         │
                       │  - 执行 Dumpling 导出                    │
                       │  - 上传到 S3                             │
                       │  - 更新任务状态                          │
                       └─────────────────────────────────────────┘
```

## API 概览

> 📖 **详细 API 文档**: 请参阅 [docs/API.md](docs/API.md)

### 管理端 API（JWT 认证）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/admin/auth/login` | POST | 管理员登录 |
| `/api/v1/admin/auth/refresh` | POST | 刷新 Token |
| `/api/v1/admin/tasks` | GET | 任务列表 |
| `/api/v1/admin/tasks/:id` | GET | 任务详情 |
| `/api/v1/admin/tasks/:id/cancel` | POST | 取消任务 |
| `/api/v1/admin/tenants` | GET/POST | 租户列表/创建 |
| `/api/v1/admin/tidb-configs` | GET/POST | TiDB 配置 |
| `/api/v1/admin/s3-configs` | GET/POST | S3 配置 |
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
    "s3_config_name": "默认存储",
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

## 配置说明

### 必须配置的环境变量

| 变量 | 说明 | 示例 |
|------|------|------|
| `HOST_DUMPLING_PATH` | Dumpling 可执行文件路径 | `/home/work/.tiup/components/dumpling/v8.5.5/dumpling` |
| `JWT_SECRET` | JWT 签名密钥（至少 32 字符） | 随机生成 |
| `AES_KEY` | AES-256 加密密钥（必须 32 字节） | `openssl rand -base64 32 \| head -c 32` |

### 可选配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `WORKER_COUNT` | 4 | Worker 并发数 |
| `S3_*` | - | 默认 S3 配置（租户可自行配置） |

## 安全特性

- **数据加密** - 敏感数据使用 AES-256-GCM 加密存储
- **双重认证** - JWT Token + API Key/Secret 机制
- **防护措施** - SQL 注入防护、时序攻击防护
- **访问限制** - 令牌桶 + 滑动窗口限流算法
- **审计日志** - 完整的操作日志记录
- **配额管理** - 租户级别的并发和容量限制

## 技术栈

### 后端
- Go 1.25+ / Gin Framework
- MySQL 8.0
- Redis 7
- Dumpling (TiDB 数据导出工具)

### 前端
- React 18 + TypeScript
- Vite 构建工具
- Ant Design 5 组件库
- Zustand 状态管理

## 常见问题

### Dumpling 找不到

```bash
# 确认 Dumpling 路径
which dumpling || tiup list --installed | grep dumpling

# 更新 .env
echo "HOST_DUMPLING_PATH=/path/to/dumpling" >> .env
```

### 数据库字段缺失

```bash
# 执行迁移
./deploy.sh migrate
```

### Worker 无法执行 Dumpling

```bash
# 验证容器内 dumpling
docker exec claw-worker /usr/local/bin/dumpling --version

# 如果报错，重新构建镜像
docker-compose build --no-cache worker
docker-compose up -d worker
```

更多问题请参阅 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)

## 许可证

MIT License
