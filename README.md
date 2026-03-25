# TiDataExporter

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![React](https://img.shields.io/badge/React-18+-61DAFB?style=flat&logo=react)](https://reactjs.org/)
[![TiDB](https://img.shields.io/badge/TiDB-Compatible-red?style=flat)](https://github.com/pingcap/tidb)

**English** | [中文](#中文文档)

A multi-tenant data export platform for TiDB, powered by [Dumpling](https://docs.pingcap.com/tidb/dev/dumpling-overview). Export your TiDB data to S3-compatible storage with ease.

## Features

- **Multi-tenant Architecture** - Complete tenant isolation with quota management
- **Flexible Export** - Support CSV, SQL formats with Gzip/Snappy/Zstd compression
- **S3 Integration** - Direct upload to various S3-compatible storage (AWS S3, MinIO, Aliyun OSS, etc.)
- **High Performance** - Built on Dumpling for efficient TiDB data export
- **Dual Authentication** - JWT for admin panel + API Key/Secret for programmatic access
- **Async Task Queue** - Redis-based task queue with priority support and auto-retry
- **Modern Admin UI** - React-based management dashboard
- **Monitoring & Auditing** - Built-in statistics reports and audit logs

## Quick Start

### Prerequisites

- Docker 20.10+
- Docker Compose 1.18.0+
- Dumpling (install via TiUP)

### Install Dumpling

```bash
# Install via TiUP
tiup install dumpling

# Verify installation
tiup list --installed | grep dumpling
```

### Deploy

```bash
# 1. Clone the repository
git clone https://github.com/Vincenzo-Wang/TiDataExporter.git
cd TiDataExporter

# 2. Copy environment configuration
cp .env.example .env

# 3. Edit configuration (Important!)
vim .env
# Must configure:
# - HOST_DUMPLING_PATH: Path to Dumpling executable
# - JWT_SECRET: JWT signing secret (at least 32 characters)
# - AES_KEY: AES-256 encryption key (exactly 32 bytes)

# 4. Start services
chmod +x deploy.sh
./deploy.sh start

# 5. Access the application
# Frontend: http://localhost
# Backend API: http://localhost:8080
# Default admin: admin / admin123 (Please change after first login!)
```

### Deploy Commands

```bash
./deploy.sh start       # Start all services
./deploy.sh stop        # Stop all services
./deploy.sh restart     # Restart all services
./deploy.sh status      # View service status
./deploy.sh logs        # View logs
./deploy.sh migrate     # Run database migrations
./deploy.sh backup      # Backup database
./deploy.sh help        # Show help
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Client Request                            │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  Frontend (Nginx)                                               │
│  - Static file serving                                          │
│  - /api reverse proxy to backend                                │
│  - Port: 80                                                     │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  Backend (Go API)                                               │
│  - JWT Auth / API Key Auth                                      │
│  - Tenant management, Task management, Config management         │
│  - Task queue (Redis)                                           │
│  - Port: 8080                                                   │
└────────┬───────────────────────┬─────────────────────────────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐    ┌─────────────────────────────────────────┐
│  MySQL          │    │  Redis                                  │
│  - Business     │    │  - Task queue (priority queue)          │
│    data storage │    │  - Cache                                │
│  - Port: 3306   │    │  - Port: 6379                           │
└─────────────────┘    └───────────────────┬─────────────────────┘
                                           │
                                           ▼
                       ┌─────────────────────────────────────────┐
                       │  Worker (Go)                            │
                       │  - Fetch tasks from queue               │
                       │  - Execute Dumpling export              │
                       │  - Upload to S3                         │
                       │  - Update task status                   │
                       └─────────────────────────────────────────┘
```

## API Overview

### Admin API (JWT Authentication)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/admin/auth/login` | POST | Admin login |
| `/api/v1/admin/auth/refresh` | POST | Refresh token |
| `/api/v1/admin/tasks` | GET | Task list |
| `/api/v1/admin/tasks/:id` | GET | Task details |
| `/api/v1/admin/tasks/:id/cancel` | POST | Cancel task |
| `/api/v1/admin/tenants` | GET/POST | Tenant list/create |
| `/api/v1/admin/tidb-configs` | GET/POST | TiDB configs |
| `/api/v1/admin/s3-configs` | GET/POST | S3 configs |
| `/api/v1/admin/statistics/overview` | GET | Statistics overview |

### Export API (API Key/Secret Authentication)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/export/tasks` | POST | Create export task |
| `/api/v1/export/tasks/:id` | GET | Query task status |
| `/api/v1/export/tasks/:id/cancel` | POST | Cancel task |

### Create Export Task

```bash
curl -X POST http://localhost:8080/api/v1/export/tasks \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -H "X-API-Secret: your-api-secret" \
  -d '{
    "tidb_config_name": "Production",
    "s3_config_name": "Default Storage",
    "sql_text": "SELECT * FROM orders WHERE created_at >= '\''2025-01-01'\''",
    "filetype": "csv",
    "compress": "gzip",
    "retention_hours": 168
  }'
```

## Configuration

### Required Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `HOST_DUMPLING_PATH` | Dumpling executable path | `/home/user/.tiup/components/dumpling/v8.5.5/dumpling` |
| `JWT_SECRET` | JWT signing secret (min 32 chars) | Random string |
| `AES_KEY` | AES-256 encryption key (exactly 32 bytes) | `openssl rand -base64 32 \| head -c 32` |

### Optional Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `WORKER_COUNT` | 4 | Worker concurrency |
| `S3_*` | - | Default S3 config (tenants can override) |

## Security

- **Data Encryption** - Sensitive data encrypted with AES-256-GCM
- **Dual Authentication** - JWT Token + API Key/Secret mechanism
- **Protection** - SQL injection protection, timing attack prevention
- **Rate Limiting** - Token bucket + sliding window rate limiting
- **Audit Logging** - Complete operation log recording
- **Quota Management** - Tenant-level concurrency and capacity limits

## Tech Stack

### Backend
- Go 1.25+ / Gin Framework
- MySQL 8.0
- Redis 7
- Dumpling (TiDB data export tool)

### Frontend
- React 18 + TypeScript
- Vite
- Ant Design 5
- Zustand

## Documentation

- [API Documentation](docs/API.md)
- [Development Guide](docs/DEVELOPMENT.md)
- [API Usage Guide](docs/api-usage-guide.md)

## Contributing

We welcome contributions! Please see our contributing guidelines:

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Setup

```bash
# Backend
cd backend
go mod download
cp config/config.yaml.example config/config.yaml
go run cmd/server/main.go

# Frontend
cd frontend
npm install
npm run dev  # http://localhost:3000
```

## Roadmap

- [ ] WebSocket real-time task status push
- [ ] Support more export formats (JSON, Excel)
- [ ] Data masking functionality
- [ ] Custom SQL templates
- [ ] Prometheus monitoring metrics
- [ ] Multi-language support (i18n)
- [ ] Swagger/OpenAPI integration

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [TiDB](https://github.com/pingcap/tidb) - Distributed SQL database
- [Dumpling](https://docs.pingcap.com/tidb/dev/dumpling-overview) - TiDB data export tool
- [TiUP](https://github.com/pingcap/tiup) - TiDB component manager

---

# 中文文档

一个基于 [Dumpling](https://docs.pingcap.com/tidb/dev/dumpling-overview) 的多租户 TiDB 数据导出平台，支持将数据导出到 S3 兼容存储。

## 功能特性

- **多租户架构** - 完整的租户隔离和配额管理
- **灵活导出** - 支持 CSV、SQL 格式，支持 Gzip/Snappy/Zstd 压缩
- **S3 集成** - 直接上传到各种 S3 兼容存储（AWS S3、MinIO、阿里云 OSS 等）
- **高性能** - 基于 Dumpling 的 TiDB 数据导出
- **双重认证** - JWT + API Key/Secret 认证机制
- **异步任务** - 基于 Redis 的任务队列，支持优先级和自动重试
- **管理后台** - 现代化的 React 管理界面
- **监控审计** - 内置统计报表和审计日志

## 快速开始

### 环境要求

- Docker 20.10+
- Docker Compose 1.18.0+
- Dumpling（通过 TiUP 安装）

### 安装 Dumpling

```bash
# 使用 TiUP 安装
tiup install dumpling

# 确认安装路径
tiup list --installed | grep dumpling
```

### 一键部署

```bash
# 1. 克隆项目
git clone https://github.com/Vincenzo-Wang/TiDataExporter.git
cd TiDataExporter

# 2. 复制环境变量配置
cp .env.example .env

# 3. 修改配置（重要！）
vim .env
# 必须配置：
# - HOST_DUMPLING_PATH：Dumpling 可执行文件路径
# - JWT_SECRET：JWT 签名密钥（至少 32 字符）
# - AES_KEY：AES-256 加密密钥（必须 32 字节）

# 4. 一键启动
chmod +x deploy.sh
./deploy.sh start

# 5. 访问应用
# 前端：http://localhost
# 后端：http://localhost:8080
# 默认管理员：admin / admin123（首次登录后请立即修改！）
```

## API 概览

### 管理端 API（JWT 认证）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/admin/auth/login` | POST | 管理员登录 |
| `/api/v1/admin/tasks` | GET | 任务列表 |
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

## 文档

- [API 接口文档](docs/API.md)
- [开发部署文档](docs/DEVELOPMENT.md)
- [API 使用指南](docs/api-usage-guide.md)

## 参与贡献

欢迎参与贡献！请遵循以下步骤：

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 提交 Pull Request

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 致谢

- [TiDB](https://github.com/pingcap/tidb) - 分布式 SQL 数据库
- [Dumpling](https://docs.pingcap.com/tidb/dev/dumpling-overview) - TiDB 数据导出工具
- [TiUP](https://github.com/pingcap/tiup) - TiDB 组件管理器
