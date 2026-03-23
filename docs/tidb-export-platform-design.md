# TiDB 数据导出平台设计文档

**项目名称：** Claw Export Platform
**版本：** 1.0.0
**创建日期：** 2026-03-23
**设计者：** AI Assistant

---

## 1. 项目概述

### 1.1 项目背景
支付系统已全面使用 TiDB 数据库，需要设计一个通用的数据导出平台，适配所有使用 TiDB 的项目。平台提供管理后台配置导出参数和 S3 云存储参数，并通过开放 API 供业务系统调用，实现 SQL 查询导出并返回文件或 OSS 下载地址。

### 1.2 核心需求
- 支持多租户（多个业务系统）独立配置 TiDB 连接和 S3 存储
- 提供管理后台：任务列表、日志详情、统计报表、配置管理
- 提供 RESTful API：创建导出任务、查询任务状态、获取文件下载链接
- 支持异步任务执行，业务系统通过轮询获取结果
- 支持业务系统指定文件保留时间，过期自动删除

### 1.3 设计原则
- **简单优先：** 采用轻量级架构，快速交付
- **易于扩展：** 为未来升级预留接口
- **安全可靠：** 多租户数据隔离，完善的认证鉴权
- **可观测性：** 完善的监控、日志和告警机制

---

## 2. 系统架构

### 2.1 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      管理后台                               │
│              任务列表 | 日志详情 | 统计报表 | 配置管理        │
└──────────────────────────┬──────────────────────────────────┘
                           │ HTTP API
┌──────────────────────────▼──────────────────────────────────┐
│                    API Gateway (Go)                         │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────┐   │
│  │  认证中间件  │  │  任务提交   │  │   任务状态查询      │   │
│  │  API Key    │  │  创建任务   │  │   轮询接口          │   │
│  └─────────────┘  └─────────────┘  └───────────────────┘   │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
                  ┌────────────────┐
                  │   TiDB Store   │  ← 元数据库
                  │   (MySQL 协议) │     - 租户配置
                  └────────────────┘     - 任务记录
                                         - 执行日志
                           │
                           ▼
                  ┌────────────────┐
                  │  Redis Stream  │  ← 任务队列
                  └────────────────┘
                           │
                           ▼
          ┌────────────────┴────────────────┐
          │     Worker Pool (Go)            │
          │  ┌─────────────────────────┐    │
          │  │  Worker 1                │    │
          │  │  - 获取任务              │    │
          │  │  - 调用 Dumpling         │    │
          │  │  - 上传 S3               │    │
          │  │  - 更新状态              │    │
          │  └─────────────────────────┘    │
          │  ┌─────────────────────────┐    │
          │  │  Worker 2                │    │
          │  └─────────────────────────┘    │
          │  ┌─────────────────────────┐    │
          │  │  Worker N (可扩展)        │    │
          │  └─────────────────────────┘    │
          └────────────────────────────────┘
                           │
                           ▼
                  ┌────────────────┐
                  │  TiDB Cluster  │  ← 业务数据库
                  │  (多个租户)     │     - 租户A数据
                  └────────────────┘     - 租户B数据
                           │
                           ▼
                  ┌────────────────┐
                  │    S3 Storage   │  ← 文件存储
                  └────────────────┘
```

### 2.2 核心组件

**部署模式说明：**
- **单进程部署（推荐用于开发和小规模生产）：** API Gateway 和 Worker Pool 运行在同一个 Go 进程中
- **多进程部署（推荐用于大规模生产）：** API Gateway 和 Worker Pool 分别部署，通过 Redis Stream 通信

**配置服务：**
- 租户配置（TiDB/S3）通过配置缓存管理（Redis，TTL 30分钟）
- 配置更新后通过 Redis Pub/Sub 通知相关组件
- 支持配置热更新，无需重启服务

**API Gateway（Go）：**
- 提供 RESTful API 接口
- 处理认证和鉴权
- 任务创建和状态查询
- 推送任务到 Redis Stream
- 配置管理和缓存

**Worker Pool（Go）：**
- 从 Redis Stream 消费任务
- 调用 Dumpling 执行导出
- 上传文件到 S3
- 更新任务状态和日志
- 监听配置变更通知

**管理后台（React）：**
- 管理员登录和认证（JWT Token）
- 任务列表和详情查看
- 执行日志查看
- 统计报表展示
- 租户配置管理（TiDB/S3/模板）
- 管理员权限管理（RBAC）

**基础设施：**
- TiDB：元数据库
- Redis：消息队列 + 配置缓存
- S3：文件存储

---

## 3. 数据模型设计

### 3.1 核心表结构

#### 管理员表（admins）
```sql
CREATE TABLE admins (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL COMMENT 'bcrypt哈希',
    email VARCHAR(100),
    role VARCHAR(20) DEFAULT 'admin' COMMENT 'admin/operator',
    status TINYINT DEFAULT 1 COMMENT '1=启用 0=禁用',
    last_login_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_status (status)
) ENGINE=InnoDB;
```

#### 租户表（tenants）
```sql
CREATE TABLE tenants (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL,
    api_key VARCHAR(64) UNIQUE NOT NULL,
    api_secret_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    status TINYINT DEFAULT 1 COMMENT '1=启用 0=禁用',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL COMMENT '软删除标记',
    INDEX idx_status (status),
    INDEX idx_api_key (api_key),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB;
```

#### 租户配额表（tenant_quotas）
```sql
CREATE TABLE tenant_quotas (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    max_concurrent_tasks INT DEFAULT 5 COMMENT '最大并发任务数',
    max_daily_tasks INT DEFAULT 100 COMMENT '每日最大任务数',
    max_daily_size_gb DECIMAL(10,2) DEFAULT 50 COMMENT '每日最大导出量(GB)',
    max_retention_hours INT DEFAULT 720 COMMENT '最长文件保留时间(小时)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    UNIQUE KEY uk_tenant_id (tenant_id)
) ENGINE=InnoDB;
```

#### TiDB 连接配置表（tidb_configs）
```sql
CREATE TABLE tidb_configs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL COMMENT '配置名称',
    host VARCHAR(255) NOT NULL,
    port INT DEFAULT 4000,
    username VARCHAR(100) NOT NULL,
    password_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    database VARCHAR(100),
    is_default TINYINT DEFAULT 0 COMMENT '是否为默认配置',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL COMMENT '软删除标记',
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_tenant_default (tenant_id, is_default),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB;
```

#### S3 配置表（s3_configs）
```sql
CREATE TABLE s3_configs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL COMMENT '配置名称',
    endpoint VARCHAR(255) NOT NULL COMMENT 'endpoint, 如s3.amazonaws.com',
    access_key VARCHAR(255) NOT NULL,
    secret_key_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    bucket VARCHAR(100) NOT NULL,
    region VARCHAR(50),
    path_prefix VARCHAR(255) DEFAULT '' COMMENT '路径前缀',
    is_default TINYINT DEFAULT 0 COMMENT '是否为默认配置',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL COMMENT '软删除标记',
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_tenant_default (tenant_id, is_default),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB;
```

#### 导出任务表（export_tasks）
```sql
CREATE TABLE export_tasks (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    task_name VARCHAR(255),
    tidb_config_id BIGINT NOT NULL,
    s3_config_id BIGINT NOT NULL,
    sql_text TEXT NOT NULL,
    filetype VARCHAR(10) DEFAULT 'sql' COMMENT 'sql/csv',
    compress VARCHAR(20) COMMENT 'gzip/snappy/zstd',
    retention_hours INT DEFAULT 168 COMMENT '文件保留时间（小时）',
    priority INT DEFAULT 5 COMMENT '任务优先级 1-10, 10最高',
    status VARCHAR(20) DEFAULT 'pending' COMMENT 'pending/running/success/failed/canceled/expired',
    file_url VARCHAR(500) COMMENT 'S3下载地址',
    file_size BIGINT COMMENT '文件大小（字节）',
    row_count BIGINT COMMENT '导出行数',
    error_message TEXT,
    cancel_reason TEXT COMMENT '取消原因',
    retry_count INT DEFAULT 0 COMMENT '重试次数',
    max_retries INT DEFAULT 3 COMMENT '最大重试次数',
    started_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,
    expires_at TIMESTAMP NULL COMMENT '文件过期时间',
    canceled_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (tidb_config_id) REFERENCES tidb_configs(id),
    FOREIGN KEY (s3_config_id) REFERENCES s3_configs(id),
    INDEX idx_tenant_status (tenant_id, status),
    INDEX idx_priority (priority),
    INDEX idx_created_at (created_at),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB;
```

#### 任务执行日志表（task_logs）
```sql
CREATE TABLE task_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL,
    log_level VARCHAR(10) COMMENT 'INFO/ERROR/WARN',
    message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES export_tasks(id) ON DELETE CASCADE,
    INDEX idx_task_created (task_id, created_at)
) ENGINE=InnoDB;
```

#### 审计日志表（audit_logs）
```sql
CREATE TABLE audit_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT,
    admin_id BIGINT,
    action VARCHAR(50) NOT NULL COMMENT '操作类型',
    resource_type VARCHAR(50) COMMENT '资源类型',
    resource_id BIGINT COMMENT '资源ID',
    request_ip VARCHAR(45),
    user_agent VARCHAR(255),
    request_data TEXT COMMENT '请求数据(脱敏)',
    result VARCHAR(20) COMMENT 'success/failed',
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tenant_created (tenant_id, created_at),
    INDEX idx_admin_created (admin_id, created_at),
    INDEX idx_resource (resource_type, resource_id)
) ENGINE=InnoDB;
```

#### Dumpling 参数模板表（dumpling_templates）
```sql
CREATE TABLE dumpling_templates (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    threads INT DEFAULT 4,
    rows_per_file INT DEFAULT 0 COMMENT '表内并发，0=关闭',
    file_size VARCHAR(20) DEFAULT '256MiB',
    consistency VARCHAR(20) DEFAULT 'auto',
    filter_rule JSON COMMENT '过滤规则，JSON格式',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL COMMENT '软删除标记',
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB;
```

---

## 4. API 接口设计

### 4.1 开放 API（业务系统调用）

**认证方式：** HTTP Header `X-API-Key: <api_key>`, `X-API-Secret: <api_secret>`

**时间格式：** 所有时间字段使用 RFC3339 格式（如：2026-03-23T13:58:00Z）

**错误响应格式：**
```json
{
  "code": 40001,
  "message": "参数验证失败",
  "data": null
}
```

**错误码定义：**
- 40001: 参数验证失败
- 40101: API Key 或 Secret 错误
- 40102: 租户已被禁用
- 40301: 权限不足
- 40401: 资源不存在
- 40901: 配额超限（并发任务数超限等）
- 42901: 请求频率超限
- 50001: 服务器内部错误

#### 创建导出任务
```
POST /api/v1/export/tasks
Content-Type: application/json
X-API-Key: <api_key>
X-API-Secret: <api_secret>

请求体：
{
  "tidb_config_name": "生产环境",
  "s3_config_name": "主存储桶",
  "sql_text": "SELECT * FROM orders WHERE created_at >= '2025-01-01'",
  "filetype": "csv",
  "compress": "gzip",
  "retention_hours": 168,
  "task_name": "2025年1月订单导出",
  "priority": 5
}

成功响应（202 Accepted）：
{
  "code": 0,
  "message": "任务已创建",
  "data": {
    "task_id": "12345678",
    "status": "pending",
    "created_at": "2026-03-23T13:58:00Z"
  }
}

错误响应（400 Bad Request）：
{
  "code": 40001,
  "message": "SQL语句包含非法关键字",
  "data": {
    "field": "sql_text",
    "error": "不允许使用DROP语句"
  }
}
```

#### 查询任务状态
```
GET /api/v1/export/tasks/{task_id}
X-API-Key: <api_key>
X-API-Secret: <api_secret>

成功响应（200 OK）：
{
  "code": 0,
  "data": {
    "task_id": "12345678",
    "task_name": "2025年1月订单导出",
    "status": "success",
    "file_url": "https://s3.amazonaws.com/bucket/exports/12345678.csv.gz",
    "file_size": 52428800,
    "row_count": 100000,
    "started_at": "2026-03-23T13:58:05Z",
    "completed_at": "2026-03-23T14:00:10Z",
    "expires_at": "2026-03-30T14:00:10Z"
  }
}
```

#### 取消任务
```
DELETE /api/v1/export/tasks/{task_id}
X-API-Key: <api_key>
X-API-Secret: <api_secret>

成功响应（200 OK）：
{
  "code": 0,
  "message": "任务已取消",
  "data": {
    "task_id": "12345678",
    "status": "canceled"
  }
}

错误响应（409 Conflict）：
{
  "code": 40901,
  "message": "任务正在执行中，无法取消",
  "data": null
}
```

#### 批量查询任务状态
```
POST /api/v1/export/tasks/batch
Content-Type: application/json
X-API-Key: <api_key>
X-API-Secret: <api_secret>

请求体：
{
  "task_ids": ["12345678", "12345679", "12345680"]
}

成功响应（200 OK）：
{
  "code": 0,
  "data": [
    {
      "task_id": "12345678",
      "status": "success",
      "file_url": "https://s3.amazonaws.com/bucket/exports/12345678.csv.gz"
    },
    {
      "task_id": "12345679",
      "status": "running"
    },
    {
      "task_id": "12345680",
      "status": "failed",
      "error_message": "SQL语法错误"
    }
  ]
}
```

#### 文件下载代理
```
GET /api/v1/export/files/{task_id}
X-API-Key: <api_key>
X-API-Secret: <api_secret>

成功响应（302 Found）：
Location: https://s3.amazonaws.com/bucket/exports/12345678.csv.gz

错误响应（404 Not Found）：
{
  "code": 40401,
  "message": "文件不存在或已过期",
  "data": null
}
```

### 4.2 管理 API（管理后台调用）

**认证方式：** HTTP Header `Authorization: Bearer <jwt_token>`

#### 管理员登录
```
POST /api/v1/admin/auth/login
Content-Type: application/json

请求体：
{
  "username": "admin",
  "password": "password123"
}

成功响应（200 OK）：
{
  "code": 0,
  "message": "登录成功",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 7200,
    "user": {
      "id": 1,
      "username": "admin",
      "role": "admin"
    }
  }
}
```

#### 刷新Token
```
POST /api/v1/admin/auth/refresh
Content-Type: application/json
Authorization: Bearer <jwt_token>

成功响应（200 OK）：
{
  "code": 0,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 7200
  }
}
```

#### 任务列表
```
GET /api/v1/admin/tasks?page=1&page_size=20&status=success&tenant_id=1&start_date=2026-03-01&end_date=2026-03-31

成功响应：
{
  "code": 0,
  "data": {
    "total": 100,
    "page": 1,
    "page_size": 20,
    "items": [
      {
        "task_id": "12345678",
        "tenant_name": "支付系统",
        "task_name": "2025年1月订单导出",
        "status": "success",
        "file_size": 52428800,
        "row_count": 100000,
        "created_at": "2026-03-23T13:58:00Z",
        "completed_at": "2026-03-23T14:00:10Z"
      }
    ]
  }
}
```

#### 任务日志详情
```
GET /api/v1/admin/tasks/{task_id}/logs

成功响应：
{
  "code": 0,
  "data": {
    "task_id": "12345678",
    "logs": [
      {
        "level": "INFO",
        "message": "开始执行任务",
        "created_at": "2026-03-23T13:58:05Z"
      },
      {
        "level": "INFO",
        "message": "Dumpling 命令: tiup dumpling -u root -P 4000 -h 127.0.0.1 ...",
        "created_at": "2026-03-23T13:58:06Z"
      }
    ]
  }
}
```

#### 统计报表
```
GET /api/v1/admin/statistics?start_date=2026-03-01&end_date=2026-03-31
Authorization: Bearer <jwt_token>

成功响应：
{
  "code": 0,
  "data": {
    "total_tasks": 500,
    "success_count": 450,
    "failed_count": 50,
    "canceled_count": 0,
    "success_rate": 90.0,
    "total_file_size": 53687091200,
    "by_tenant": [
      {
        "tenant_name": "支付系统",
        "task_count": 300,
        "success_rate": 95.0
      }
    ],
    "by_date": [
      {
        "date": "2026-03-01",
        "count": 20,
        "success_rate": 92.0
      }
    ]
  }
}
```

#### 租户管理 - 创建租户
```
POST /api/v1/admin/tenants
Content-Type: application/json
Authorization: Bearer <jwt_token>

请求体：
{
  "name": "新业务系统",
  "status": 1
}

成功响应（201 Created）：
{
  "code": 0,
  "message": "租户创建成功",
  "data": {
    "tenant_id": 2,
    "api_key": "sk_live_1234567890abcdef",
    "api_secret": "sk_live_secret_1234567890abcdef",
    "name": "新业务系统",
    "status": 1,
    "created_at": "2026-03-23T13:58:00Z"
  }
}

注意：api_secret 只在创建时返回一次，请妥善保存
```

#### 租户管理 - 列表
```
GET /api/v1/admin/tenants?page=1&page_size=20
Authorization: Bearer <jwt_token>

成功响应：
{
  "code": 0,
  "data": {
    "total": 10,
    "page": 1,
    "page_size": 20,
    "items": [
      {
        "tenant_id": 1,
        "name": "支付系统",
        "api_key": "sk_live_1234567890abcdef",
        "status": 1,
        "created_at": "2026-03-23T13:58:00Z"
      }
    ]
  }
}
```

#### TiDB 配置管理 - 创建
```
POST /api/v1/admin/tidb-configs
Content-Type: application/json
Authorization: Bearer <jwt_token>

请求体：
{
  "tenant_id": 1,
  "name": "生产环境",
  "host": "127.0.0.1",
  "port": 4000,
  "username": "root",
  "password": "password123",
  "database": "payment",
  "is_default": true
}

成功响应（201 Created）：
{
  "code": 0,
  "message": "TiDB配置创建成功",
  "data": {
    "config_id": 1,
    "tenant_id": 1,
    "name": "生产环境",
    "host": "127.0.0.1",
    "port": 4000,
    "username": "root",
    "is_default": true,
    "created_at": "2026-03-23T13:58:00Z"
  }
}
```

#### TiDB 配置管理 - 列表
```
GET /api/v1/admin/tidb-configs?tenant_id=1
Authorization: Bearer <jwt_token>

成功响应：
{
  "code": 0,
  "data": {
    "total": 2,
    "items": [
      {
        "config_id": 1,
        "tenant_id": 1,
        "name": "生产环境",
        "host": "127.0.0.1",
        "port": 4000,
        "username": "root",
        "is_default": true,
        "created_at": "2026-03-23T13:58:00Z"
      }
    ]
  }
}
```

#### TiDB 配置管理 - 更新
```
PUT /api/v1/admin/tidb-configs/{config_id}
Content-Type: application/json
Authorization: Bearer <jwt_token>

请求体：
{
  "name": "生产环境（已更新）",
  "password": "newpassword123"
}

成功响应（200 OK）：
{
  "code": 0,
  "message": "TiDB配置更新成功"
}
```

#### S3 配置管理 - 创建
```
POST /api/v1/admin/s3-configs
Content-Type: application/json
Authorization: Bearer <jwt_token>

请求体：
{
  "tenant_id": 1,
  "name": "主存储桶",
  "endpoint": "s3.amazonaws.com",
  "access_key": "AKIAIOSFODNN7EXAMPLE",
  "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  "bucket": "my-bucket",
  "region": "us-east-1",
  "path_prefix": "/exports/tenant1/",
  "is_default": true
}

成功响应（201 Created）：
{
  "code": 0,
  "message": "S3配置创建成功",
  "data": {
    "config_id": 1,
    "tenant_id": 1,
    "name": "主存储桶",
    "endpoint": "s3.amazonaws.com",
    "access_key": "AKIAIOSFODNN7EXAMPLE",
    "bucket": "my-bucket",
    "region": "us-east-1",
    "is_default": true,
    "created_at": "2026-03-23T13:58:00Z"
  }
}
```

#### S3 配置管理 - 列表
```
GET /api/v1/admin/s3-configs?tenant_id=1
Authorization: Bearer <jwt_token>

成功响应：
{
  "code": 0,
  "data": {
    "total": 1,
    "items": [
      {
        "config_id": 1,
        "tenant_id": 1,
        "name": "主存储桶",
        "endpoint": "s3.amazonaws.com",
        "access_key": "AKIAIOSFODNN7EXAMPLE",
        "bucket": "my-bucket",
        "region": "us-east-1",
        "is_default": true,
        "created_at": "2026-03-23T13:58:00Z"
      }
    ]
  }
}
```

#### Dumpling 模板管理 - 创建
```
POST /api/v1/admin/dumpling-templates
Content-Type: application/json
Authorization: Bearer <jwt_token>

请求体：
{
  "tenant_id": 1,
  "name": "大表导出模板",
  "threads": 8,
  "rows_per_file": 200000,
  "file_size": "512MiB",
  "consistency": "snapshot",
  "filter_rule": {
    "exclude": ["information_schema", "performance_schema", "mysql"]
  }
}

成功响应（201 Created）：
{
  "code": 0,
  "message": "Dumpling模板创建成功",
  "data": {
    "template_id": 1,
    "tenant_id": 1,
    "name": "大表导出模板",
    "threads": 8,
    "rows_per_file": 200000,
    "file_size": "512MiB",
    "consistency": "snapshot",
    "created_at": "2026-03-23T13:58:00Z"
  }
}
```

#### 健康检查
```
GET /health

成功响应（200 OK）：
{
  "status": "healthy",
  "components": {
    "database": "ok",
    "redis": "ok",
    "s3": "ok"
  },
  "version": "1.0.0"
}
```

#### 就绪检查
```
GET /health/ready

成功响应（200 OK）：
{
  "status": "ready",
  "workers": {
    "active": 4,
    "total": 4
  }
}
```

---

## 5. 核心业务流程

### 5.1 任务执行流程

```
┌─────────────┐
│ 业务系统     │
└──────┬──────┘
       │ 1. POST /api/v1/export/tasks
       │    {sql_text, config_names, ...}
       ▼
┌─────────────────────────────────────────────────────────┐
│ API Gateway                                              │
│                                                          │
│ 1. 认证校验（API Key/Secret）                           │
│ 2. 参数验证                                            │
│ 3. 查询租户配置（TiDB/S3）                               │
│ 4. 创建任务记录（status=pending）                        │
│ 5. 推送任务到 Redis Stream                             │
│    Stream: "export:tasks"                              │
│    Message: {task_id, tenant_id, configs, sql, ...}   │
│ 6. 返回 task_id 给业务系统                              │
└─────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────┐
│ Redis Stream (export:tasks)                             │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Message 1: {task_id: 1, status: pending}       │   │
│  │ Message 2: {task_id: 2, status: pending}       │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────┐
│ Worker Pool (消费组: export-workers)                     │
│                                                          │
│ Worker 获取任务：                                        │
│ 1. 从 Redis Stream XREADGROUP 读取消息                   │
│ 2. 更新任务状态：running, 记录 started_at                │
│ 3. 准备 Dumpling 参数：                                  │
│    - 连接 TiDB                                           │
│    - 生成 S3 输出路径: s3://bucket/exports/{task_id}/   │
│    - 构建命令: tiup dumpling --sql="{sql}" -o "{path}" │
│                                                          │
│ 4. 执行导出：                                            │
│    - 调用 Dumpling CLI                                  │
│    - 实时捕获日志写入 task_logs 表                       │
│                                                          │
│ 5. 上传到 S3：                                          │
│    - 如果 Dumpling 输出到本地，使用 AWS SDK 上传         │
│    - 或者直接配置 Dumpling 输出到 S3 URI                 │
│    - 记录 file_url, file_size, row_count                │
│                                                          │
│ 6. 更新任务状态：success/failed                          │
│    - 成功：记录 completed_at, expires_at                 │
│    - 失败：记录 error_message                            │
│                                                          │
│ 7. ACK Redis Stream 消息                                │
└─────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────┐
│ 业务系统     │
│             │
│ 轮询查询：   │
│ GET /api/v1/export/tasks/{task_id}                      │
│             │
│ 状态变化：   │
│ pending → running → success → 返回 file_url            │
└─────────────┘
```

### 5.2 任务取消流程

```
┌─────────────┐
│ 业务系统     │
└──────┬──────┘
       │ DELETE /api/v1/export/tasks/{task_id}
       ▼
┌─────────────────────────────────────────────────────────┐
│ API Gateway                                              │
│                                                          │
│ 1. 认证校验                                              │
│ 2. 查询任务状态                                          │
│ 3. 判断是否可取消：                                       │
│    - pending：可以取消                                   │
│    - running：检查 Dumpling 进程，尝试终止               │
│    - success/failed/canceled：返回错误                    │
│ 4. 更新任务状态：canceled, 记录 cancel_reason           │
│ 5. 如果任务在 running 状态：                             │
│    - 发送取消信号到 Worker                               │
│    - Worker 终止 Dumpling 进程                           │
│ 6. ACK Redis Stream 消息                                │
└─────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────┐
│ Worker Pool                                              │
│                                                          │
│ Worker 处理取消请求：                                     │
│ 1. 查找正在执行的 Dumpling 进程                          │
│ 2. 发送 SIGTERM 信号给进程                               │
│ 3. 等待 30 秒，如果未退出则 SIGKILL                     │
│ 4. 清理已生成的临时文件：                                 │
│    - 本地文件：删除                                      │
│    - S3 文件：如果已上传，删除 S3 上的部分文件           │
│ 5. 更新任务状态：canceled                               │
│ 6. 记录取消日志到 task_logs                             │
└─────────────────────────────────────────────────────────┘
```

### 5.3 任务重试流程

```
┌─────────────────────────────────────────────────────────┐
│ Worker Pool                                              │
│                                                          │
│ Worker 处理失败任务：                                     │
│ 1. 检查重试次数（retry_count < max_retries）            │
│ 2. 判断是否可重试：                                     │
│    - Dumpling 执行失败：可重试                           │
│    - S3 上传失败：可重试                               │
│    - SQL 语法错误：不可重试                             │
│    - 配置错误：不可重试                                 │
│ 3. 如果可重试：                                         │
│    - increment retry_count                             │
│    - 延迟重试（指数退避：2^retry_count 秒）            │
│    - 重新推送任务到 Redis Stream                        │
│    - 记录重试日志                                      │
│ 4. 如果不可重试：                                       │
│    - 更新任务状态：failed                               │
│    - 记录 error_message                                │
│    - 清理临时文件                                      │
└─────────────────────────────────────────────────────────┘
```

### 5.4 任务失败清理流程

```
┌─────────────────────────────────────────────────────────┐
│ Worker Pool                                              │
│                                                          │
│ 任务执行失败后的清理：                                     │
│ 1. 检查失败类型：                                       │
│    - Dumpling 失败：清理本地输出目录                      │
│    - S3 上传失败：保留本地文件（可手动重试）             │
│ 2. 本地文件清理：                                       │
│    - 删除临时输出目录：/tmp/exports/{task_id}/         │
│ 3. S3 部分文件清理（可选）：                            │
│    - 删除 S3 上已上传的部分文件                         │
│ 4. 记录清理日志到 task_logs                            │
│ 5. 更新任务状态：failed                                │
└─────────────────────────────────────────────────────────┘

磁盘空间保护：
- 定期检查磁盘使用率
- 如果磁盘使用率 > 85%，清理超过 24 小时的失败任务本地文件
- 发送告警通知
```

### 5.5 Worker 崩溃恢复流程

```
┌─────────────────────────────────────────────────────────┐
│ Redis Stream (export:tasks)                             │
│                                                          │
│ 消息未 ACK 处理：                                        │
│ 1. XACK 超时时间：30 分钟                              │
│ 2. 消息超时后重新进入 Pending 状态                      │
│ 3. 其他 Worker 可以重新获取消息                          │
└─────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────┐
│ Worker Pool                                              │
│                                                          │
│ Worker 处理超时消息：                                     │
│ 1. 检查任务当前状态：                                   │
│    - 如果任务已完成：忽略消息                           │
│    - 如果任务在执行中：记录日志，继续执行               │
│ 2. 幂等性保证：                                         │
│    - 检查 S3 是否已有文件                               │
│    - 如果有，直接更新任务状态为 success                 │
│    - 如果没有，重新执行导出                             │
└─────────────────────────────────────────────────────────┘
```

### 5.6 任务超时处理流程

```
┌─────────────────────────────────────────────────────────┐
│ 定时任务（每分钟执行）                                   │
│                                                          │
│ 1. 查询超时任务：                                       │
│    SELECT * FROM export_tasks                           │
│    WHERE status = 'running'                              │
│      AND started_at < DATE_SUB(NOW(), INTERVAL 2 HOUR)   │
│ 2. 判断任务类型：                                       │
│    - 根据 file_size 估算预期执行时间                     │
│    - 如果确实超时，执行超时处理                         │
│ 3. 超时处理：                                           │
│    - 发送终止信号给 Worker                              │
│    - 更新任务状态：failed                              │
│    - 记录 error_message = "任务执行超时"                │
│    - 清理临时文件                                       │
└─────────────────────────────────────────────────────────┘
```

### 5.7 文件清理流程

**定时清理任务（Cron: 每小时执行一次）**

1. 查询过期任务：
```sql
SELECT * FROM export_tasks
WHERE expires_at < NOW() AND status = 'success'
```

2. 删除 S3 文件：
- 使用 AWS SDK DeleteObject API
- 删除整个目录：`s3://bucket/exports/{task_id}/`

3. 更新任务状态：
```sql
UPDATE export_tasks
SET status = 'expired'
WHERE id IN (...)
```

4. 清理历史日志（可选）：
- 删除 30 天前的 task_logs

---

## 6. 技术栈

### 6.1 后端技术栈（Go）

```
核心框架：
├── Gin Web Framework (HTTP 服务)
├── GORM (ORM，支持 TiDB/MySQL 协议)
├── go-redis/redis (Redis 客户端)
└── AWS SDK for Go v2 (S3 操作)

任务队列：
├── Redis Stream (消息队列)
└── 内置 Worker Pool (任务执行)

TiDB 集成：
├── go-sql-driver/mysql (TiDB 驱动)
└── Dumpling CLI (通过 Go 执行命令)

认证授权：
├── golang-jwt/jwt (JWT Token 生成和验证)
├── golang.org/x/crypto (bcrypt 密码哈希, AES-256 加密)
└── CSRF 中间件

监控与日志：
├── Zap (结构化日志)
├── Prometheus (指标采集，可选)
└── pprof (性能分析)

配置管理：
├── Viper (配置文件管理)
└── 环境变量支持

工具库：
├── uuid (生成 task_id)
├── validator (参数校验)
├── robfig/cron (定时任务，如文件清理)
├── vitess (SQL 解析器，用于 SQL 注入防护，可选)
└── limiter (基于 Redis 的限流器)

数据库迁移：
├── golang-migrate (数据库版本管理)
└── SQL 迁移脚本

API 文档：
└── swaggo/swag (Swagger/OpenAPI 文档生成)
```

### 6.2 前端技术栈（React）

```
框架与路由：
├── React 18
├── React Router v6
└── TypeScript

UI 组件库：
├── Ant Design (企业级组件库)
│   ├── Table (任务列表)
│   ├── Form (配置管理)
│   ├── Card/Dashboard (统计报表)
│   └── Modal/Drawer (详情查看)
└── ECharts 或 Recharts (数据可视化)

状态管理：
├── Zustand (轻量级状态管理)
└── React Query (服务端状态)

HTTP 客户端：
└── Axios

工具库：
├── dayjs (日期处理)
└── lodash (工具函数)

构建工具：
└── Vite
```

### 6.3 基础设施

```
存储：
├── TiDB (元数据库)
├── Redis (消息队列)
└── S3 兼容对象存储 (文件存储)

部署：
├── Docker (容器化)
├── Docker Compose (开发环境)
└── Kubernetes (生产环境，可选)
```

### 6.4 项目结构

```
claw-export-platform/
├── backend/                      # Go 后端
│   ├── api/                      # API 路由和处理器
│   │   ├── v1/
│   │   │   ├── export.go         # 开放 API
│   │   │   └── admin.go          # 管理 API
│   │   └── middleware/
│   │       └── auth.go           # 认证中间件
│   ├── models/                   # 数据模型
│   │   ├── tenant.go
│   │   ├── task.go
│   │   └── config.go
│   ├── services/                 # 业务逻辑
│   │   ├── task_service.go
│   │   ├── export_service.go
│   │   └── s3_service.go
│   ├── workers/                  # Worker Pool
│   │   ├── worker.go
│   │   └── executor.go           # Dumpling 执行器
│   ├── config/                   # 配置管理
│   │   └── config.go
│   ├── pkg/                      # 公共包
│   │   ├── redis/
│   │   ├── database/
│   │   └── logger/
│   ├── main.go
│   └── go.mod
│
├── frontend/                     # React 前端
│   ├── src/
│   │   ├── api/                  # API 客户端
│   │   ├── components/           # 组件
│   │   ├── pages/                # 页面
│   │   │   ├── Tasks.tsx
│   │   │   ├── TaskDetail.tsx
│   │   │   ├── Statistics.tsx
│   │   │   └── Config.tsx
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── package.json
│   └── vite.config.ts
│
├── deploy/                       # 部署配置
│   ├── docker/
│   │   ├── Dockerfile.backend
│   │   └── Dockerfile.frontend
│   └── docker-compose.yml
│
├── docs/                         # 文档
│   ├── api.md
│   └── architecture.md
│
└── README.md
```

---

## 7. 安全设计

### 7.1 认证与鉴权

**API Key 认证流程（业务系统）：**
```
业务系统请求 → 认证中间件 → 验证 API Key/Secret → 允许访问
                  ↓
         查询 tenants 表
         使用 AES-256 解密 api_secret_encrypted
         验证与请求中的 Secret 是否匹配
         验证 status = 1
         提取 tenant_id
         注入上下文
```

**管理员认证流程（管理后台）：**
```
管理员登录 → 验证用户名和密码 → 生成 JWT Token → 返回 Token
            ↓
      查询 admins 表
      验证 password_hash (bcrypt)
      生成 JWT Token（有效期 2 小时）
      返回 Token 给前端
```

**API Secret 存储方案：**
- **使用 AES-256 对称加密**，而非 bcrypt 哈希
- 加密密钥存储在环境变量 `ENCRYPTION_KEY` 中
- 解密流程：`api_secret = AES256_Decrypt(api_secret_encrypted, ENCRYPTION_KEY)`
- 为什么用加密而非哈希：API Secret 需要还原才能验证请求

**管理员密码存储方案：**
- **使用 bcrypt 哈希**（带 salt）
- 加密因子：10
- 验证流程：`bcrypt.CompareHashAndPassword(password_hash, password)`

**安全性措施：**

1. **API 认证：**
   - API Secret 使用 AES-256 加密存储
   - 传输使用 HTTPS（生产环境强制）
   - API Key 过期机制（可选）
   - IP 白名单（可选配置）

2. **管理后台认证：**
   - 密码使用 bcrypt 哈希存储
   - JWT Token 认证
   - Token 刷新机制
   - CSRF 防护（所有写操作需要 CSRF Token）
   - 密码策略：至少8位，包含大小写字母、数字、特殊字符
   - 密码过期：90天强制修改

3. **RBAC 权限控制：**
   - 角色：admin（管理员）、operator（操作员）
   - admin：所有权限
   - operator：查看任务、查看日志、创建配置，不能删除租户

4. **请求签名（可选，增强安全性）：**
   - 使用 HMAC-SHA256 签名请求
   - 签名算法：`signature = HMAC-SHA256(api_secret, timestamp + method + path + body)`
   - 在 HTTP Header `X-Signature` 中传递签名
   - 防止请求被截获后重放

### 7.2 数据隔离

**多租户隔离策略：**

1. **数据库层隔离：**
   - 所有表都包含 tenant_id
   - 查询自动注入 WHERE tenant_id = ?
   - 外键约束防止跨租户数据访问

2. **配置隔离：**
   - 每个 tenant 独立的 TiDB 连接配置
   - 每个 tenant 独立的 S3 配置
   - S3 路径前缀：`/exports/{tenant_id}/{task_id}/`

3. **任务隔离：**
   - Worker 执行时使用对应 tenant 的 TiDB 配置
   - S3 上传到对应 tenant 的路径
   - 任务日志按 tenant_id 索引

### 7.3 SQL 注入防护

**防护措施：**

1. **参数校验：**
   - 禁止危险的 SQL 关键字（DROP, DELETE, TRUNCATE, UPDATE, INSERT, etc.）
   - 限制 SQL 长度（最大 10KB）
   - 只允许 SELECT 语句
   - 使用 SQL 解析器进行语法验证（推荐使用 vitess 或类似库）

2. **SQL 白名单模式：**
   - 使用 SQL 解析器解析 SQL 语法树
   - 验证是否只包含 SELECT 语句
   - 检查是否包含子查询、JOIN 等潜在风险操作
   - 拒绝包含以下关键字的 SQL：
     - 修改操作：INSERT, UPDATE, DELETE, DROP, TRUNCATE, ALTER, CREATE
     - 管理操作：GRANT, REVOKE, FLUSH, RESET
     - 系统操作：KILL, SHUTDOWN

3. **Dumpling 参数化：**
   - Dumpling 接受的 SQL 应该是只读的
   - 使用 Dumpling 内置的安全机制
   - Dumpling 使用只读连接执行 SQL

4. **审计日志：**
   - 记录所有提交的 SQL（脱敏处理）
   - 敏感操作告警
   - 可疑 SQL 自动拦截并通知管理员

5. **敏感字段脱敏：**
   - API 响应中不返回完整的 sql_text
   - 只返回 SQL 的摘要（前 100 字符）
   - 管理后台查看详情需要额外权限

### 7.4 凭证管理

**敏感信息保护：**

1. **存储加密：**
   - TiDB 数据库密码使用 AES-256 加密存储（tidb_configs.password_encrypted）
   - S3 Secret Key 使用 AES-256 加密存储（s3_configs.secret_key_encrypted）
   - API Secret 使用 AES-256 加密存储（tenants.api_secret_encrypted）
   - 加密密钥存储在环境变量 `ENCRYPTION_KEY` 中（32 字节）
   - 生产环境建议使用密钥管理系统（如 AWS KMS、HashiCorp Vault）

2. **运行时保护：**
   - 配置文件不包含明文密码
   - 环境变量注入敏感信息
   - 日志脱敏（不输出密码、Secret Key）
   - 调试模式下禁止输出敏感信息

3. **S3 访问控制：**
   - 生成预签名 URL（临时访问权限）
   - URL 有效期与文件 retention_hours 一致
   - 使用 HTTPS 访问
   - 预签名 URL 包含：
     - `X-Amz-Expires`: 秒数
     - `X-Amz-Date`: 签名时间
     - `X-Amz-Credential`: 凭证信息
     - `X-Amz-Signature`: HMAC-SHA256 签名

4. **密钥轮换：**
   - 定期轮换加密密钥（建议每 6 个月）
   - 密钥轮换流程：
     1. 生成新密钥
     2. 使用新密钥重新加密所有敏感数据
     3. 更新环境变量
     4. 验证功能正常
     5. 删除旧密钥
   - API Secret 和管理员密码可手动重置

5. **密钥备份：**
   - 加密密钥需要安全备份
   - 使用多个副本存储在不同位置
   - 遵循 3-2-1 原则（3 份副本，2 种介质，1 个异地）

### 7.5 限流与配额

**防止滥用：**

1. **API 限流：**
   - 基于 Redis 的令牌桶算法
   - 每个 tenant 独立的速率限制
   - 默认配置：100 req/min
   - 限流算法：Token Bucket
   - 限流响应：HTTP 429 + `Retry-After` header

2. **任务并发限制：**
   - 每个 tenant 最大并发任务数（默认 5，可配置）
   - 创建任务时检查当前 running 任务数
   - 超限返回 HTTP 409 + 错误码 40901

3. **资源配额（基于 tenant_quotas 表）：**
   - 每日最大任务数（max_daily_tasks）
   - 每日最大导出数据量（max_daily_size_gb）
   - 最长文件保留时间（max_retention_hours）
   - 超配额返回 HTTP 409 + 错误信息

4. **IP 白名单（可选）：**
   - 可为每个租户配置允许访问的 IP 地址段
   - 不在白名单的请求直接拒绝
   - 支持 CIDR 格式（如 192.168.1.0/24）

5. **文件访问频率限制：**
   - 防止文件 URL 被滥用转发
   - 每个 IP 每分钟最多下载 10 次
   - 超限返回 HTTP 429

6. **安全审计：**
   - 记录所有敏感操作到 audit_logs 表：
     - 登录/登出
     - 配置创建/更新/删除
     - 任务创建/取消/删除
     - 租户创建/更新/删除
   - 审计日志包含：
     - 操作人（tenant_id 或 admin_id）
     - 操作类型（action）
     - 操作对象（resource_type, resource_id）
     - 请求 IP（request_ip）
     - 操作时间（created_at）
     - 操作结果（result）

---

## 8. 监控与运维

### 8.1 核心监控指标

**业务指标：**
- 任务成功率（按租户、按时间段）
- 任务执行时长分布（P50, P95, P99）
- 任务排队时间
- 导出数据量统计
- API QPS 和响应时间
- S3 存储容量

**系统指标：**
- Worker 在线数量和 CPU/内存使用率
- 任务处理速率
- 任务队列积压数量
- TiDB 连接数和慢查询
- Redis 连接数和内存使用率

### 8.2 日志规范

**日志级别：**
- DEBUG：详细的调试信息（开发环境）
- INFO：关键业务流程（任务创建、完成、失败）
- WARN：可恢复的错误（重试、降级）
- ERROR：严重的错误（任务失败、服务不可用）

**结构化日志格式（JSON）：**
```json
{
  "timestamp": "2026-03-23T13:58:00Z",
  "level": "INFO",
  "service": "api-gateway",
  "tenant_id": "123456",
  "task_id": "789012",
  "message": "任务创建成功",
  "duration_ms": 45,
  "metadata": {
    "sql_length": 256,
    "config_tidb": "生产环境"
  }
}
```

### 8.3 告警规则

**关键告警：**

**P0 - 紧急（立即通知）：**
- 服务宕机（API Gateway 或 Worker 全部不可用）
- 数据库连接失败
- 任务失败率 > 50%（持续 5 分钟）
- S3 上传失败率 > 50%（持续 10 分钟）

**P1 - 重要（30 分钟内通知）：**
- Worker 数量 < 最小阈值
- 任务队列积压 > 1000 个
- API 错误率 > 5%（持续 10 分钟）
- 磁盘空间使用率 > 80%

**P2 - 一般（1 小时内通知）：**
- 任务执行时长 > 1 小时
- API 响应时间 > 5s（P95）
- 单租户任务失败率 > 20%（持续 30 分钟）

### 8.4 部署与回滚

**部署策略：**

1. **蓝绿部署：**
   - 新版本部署到备用环境
   - 健康检查通过后切换流量
   - 出问题立即回滚

2. **滚动更新：**
   - 逐个更新 Worker 实例
   - 保证至少一个 Worker 在线

3. **数据库迁移：**
   - 使用 Flyway 或类似工具管理 schema
   - 支持向前兼容的变更

---

## 9. 性能与扩展性

### 9.1 性能优化策略

**并发控制：**
- Worker Pool 配置：默认 4-8 个 Worker
- Dumpling 参数优化：根据任务规模动态调整线程数
- S3 上传优化：使用分片上传，并行上传多个文件

**数据库优化：**
- TiDB 连接池：最大连接数 50，空闲连接数 10
- 查询优化：避免全表扫描，使用 TiDB 事务快照
- 索引优化：复合索引优化查询性能

**缓存策略：**
- 租户配置缓存：TTL 30 分钟
- 任务状态缓存：TTL 5 分钟
- 统计数据缓存：TTL 15 分钟

### 9.2 扩展性设计

**水平扩展：**
- API Gateway：无状态设计，可横向扩展
- Worker Pool：消费组模式，支持多个实例
- 存储：S3 自动扩展容量

**架构演进路径：**
1. 从 Redis Stream 到 RabbitMQ/Kafka（更强的消息可靠性）
2. 从单体到微服务（API Gateway、Worker、配置服务独立）
3. 引入分布式任务调度（支持任务依赖和复杂调度）

### 9.3 性能基准

**预期性能指标：**

小任务（<100MB）：
- 执行时长：1-5 分钟
- 文件生成：30s-2 分钟
- S3 上传：30s-3 分钟

中等任务（100MB-1GB）：
- 执行时长：5-15 分钟
- 文件生成：2-10 分钟
- S3 上传：3-5 分钟

大任务（1GB-5GB）：
- 执行时长：15-60 分钟
- 文件生成：10-45 分钟
- S3 上传：5-15 分钟

**并发能力：**
- 单实例 Worker：4-8 任务/小时
- 水平扩展后：可支持 50+ 任务/小时

---

## 10. 未来扩展方向

### 10.1 功能增强

1. **定时任务支持：**
   - 支持在管理后台配置定时导出
   - 支持 Cron 表达式
   - 任务失败自动重试

2. **模板管理：**
   - 预设常用导出模板
   - 支持参数化 SQL
   - 快速创建任务

3. **数据转换：**
   - 支持导出后进行数据转换
   - 支持自定义输出格式
   - 支持数据脱敏

4. **通知机制：**
   - 任务完成邮件通知
   - Webhook 回调
   - 钉钉/企业微信集成

### 10.2 架构升级

1. **消息队列升级：**
   - 从 Redis Stream 迁移到 RabbitMQ 或 Kafka
   - 支持任务优先级
   - 支持延迟队列

2. **微服务化：**
   - 拆分为 API Gateway、Worker、配置服务等独立服务
   - 使用 Service Mesh（Istio）
   - 实现服务治理

3. **分布式调度：**
   - 使用分布式任务调度框架（如 Temporal）
   - 支持任务依赖和编排
   - 支持跨数据中心调度

### 10.3 安全增强

1. **OAuth2 认证：**
   - 支持 OAuth2 + JWT
   - 集成企业 SSO
   - 支持多因素认证

2. **数据加密：**
   - 传输加密（TLS）
   - 存储加密（加密上传到 S3 的文件）
   - 密钥轮换机制

3. **审计增强：**
   - 完整的操作审计日志
   - 敏感操作二次确认
   - 合规性报告生成

---

## 11. 实施计划

### 11.1 开发阶段

**Phase 0 - 技术验证（1 周）**
- Dumpling CLI 集成验证
- S3 上传功能验证
- Redis Stream 消息队列验证
- TiDB 连接和数据模型验证
- 技术风险识别和解决方案

**Phase 1 - 核心功能（4 周）**
- 数据库表结构设计和迁移脚本
- API Gateway 基础框架搭建
- 认证授权模块（API Key + JWT）
- Worker Pool 和任务队列实现
- Dumpling 集成和 S3 上传
- 任务取消、重试、超时处理
- 配置缓存机制
- 基础限流和配额控制

**Phase 2 - 管理后台（3 周，与 Phase 1 部分并行）**
- React 前端框架搭建
- 管理员登录和认证（JWT + CSRF）
- 任务列表和详情页面
- 执行日志查看
- 统计报表展示
- 租户配置管理（TiDB/S3/模板）
- 管理员权限管理（RBAC）

**Phase 3 - 测试和优化（2 周）**
- 单元测试（覆盖率要求：80%+）
- 集成测试
- API 测试（使用 Postman/Newman）
- 性能测试和压力测试
- 安全测试（SQL 注入、XSS、CSRF 等）
- 代码审查和重构

**Phase 4 - 部署和上线（1 周）**
- Docker 容器化（后端 + 前端）
- Docker Compose 编排（开发环境）
- CI/CD 流水线配置
- 监控和告警配置
- 灰度发布
- 文档完善（API 文档、部署文档、运维手册）

### 11.2 总体时间表

- **Week 0：** 技术验证（关键步骤，确保技术可行性）
- **Week 1-4：** 后端核心功能开发
- **Week 2-6：** 前端管理后台开发（与后端并行 2 周）
- **Week 7-8：** 测试和优化
- **Week 9：** 部署和上线

**总计：** 约 10 周完成 MVP 版本

### 11.3 人员配置

**建议团队配置：**
- 后端开发：2 人
- 前端开发：1 人
- 测试工程师：1 人
- DevOps/运维：0.5 人（兼职）

### 11.4 验收标准

**MVP 版本必须包含的功能：**

1. **核心功能：**
   - ✅ 业务系统可以创建导出任务
   - ✅ 业务系统可以查询任务状态
   - ✅ 业务系统可以下载导出文件
   - ✅ 支持取消正在执行的任务
   - ✅ 支持任务失败自动重试
   - ✅ 文件过期自动清理

2. **管理后台：**
   - ✅ 管理员可以登录系统
   - ✅ 管理员可以查看所有任务
   - ✅ 管理员可以查看任务日志
   - ✅ 管理员可以查看统计报表
   - ✅ 管理员可以创建/编辑/删除租户
   - ✅ 管理员可以创建/编辑/删除 TiDB 配置
   - ✅ 管理员可以创建/编辑/删除 S3 配置
   - ✅ 管理员可以创建/编辑/删除 Dumpling 模板

3. **安全性：**
   - ✅ API Key 认证工作正常
   - ✅ JWT Token 认证工作正常
   - ✅ 敏感信息加密存储（API Secret、密码、密钥）
   - ✅ SQL 注入防护有效
   - ✅ 多租户数据隔离有效
   - ✅ 审计日志记录完整

4. **性能：**
   - ✅ 单实例可支持 4-8 任务/小时
   - ✅ API 响应时间 P95 < 1s
   - ✅ 任务执行时长符合预期基准

5. **质量：**
   - ✅ 单元测试覆盖率 ≥ 80%
   - ✅ 无 P0/P1 级别的严重 bug
   - ✅ API 文档完整准确
   - ✅ 部署文档清晰可操作

---

## 12. 风险与应对

### 12.1 技术风险

| 风险 | 影响 | 概率 | 应对措施 |
|------|------|------|----------|
| TiDB 性能瓶颈 | 高 | 中 | 使用连接池、优化查询、增加缓存 |
| Dumpling 执行失败 | 高 | 低 | 完善错误处理、支持重试、记录详细日志 |
| S3 上传失败 | 高 | 低 | 分片上传、断点续传、重试机制 |
| Redis 单点故障 | 中 | 低 | 使用 Redis Cluster 或哨兵模式 |

### 12.2 业务风险

| 风险 | 影响 | 概率 | 应对措施 |
|------|------|------|----------|
| 任务积压 | 中 | 低 | 动态扩容 Worker、限流保护 |
| 存储成本过高 | 中 | 中 | 定期清理过期文件、压缩存储 |
| 数据泄露 | 高 | 低 | 加密存储、访问控制、审计日志 |

---

## 13. 总结

本设计文档详细描述了 TiDB 数据导出平台的技术架构、数据模型、API 接口、业务流程、安全设计、监控运维、性能优化等方面。平台采用轻量级架构，使用 Go + React 技术栈，基于 Redis Stream 实现异步任务队列，支持多租户隔离，提供完善的监控和告警机制。

设计遵循简单优先、易于扩展、安全可靠的原则，预计在 10 周内完成 MVP 版本（包括 1 周技术验证），同时为未来升级预留了充足的扩展空间。平台将有效解决支付系统及其他 TiDB 项目的数据导出需求，提高数据导出效率和管理便捷性。

### 主要特性

1. **多租户支持：** 完整的租户隔离机制，支持多个业务系统独立配置
2. **异步任务：** 基于 Redis Stream 的异步任务队列，支持长时间运行的任务
3. **安全管理：** 多层安全防护，包括 API Key 认证、JWT 认证、数据加密、SQL 注入防护、审计日志
4. **任务管理：** 支持任务创建、查询、取消、重试、超时处理
5. **文件管理：** 自动清理过期文件，支持 S3 预签名 URL 访问
6. **可观测性：** 完善的监控、日志和告警机制
7. **可扩展性：** 为未来升级预留接口，支持从 Redis Stream 到 RabbitMQ/Kafka 的平滑迁移

### 文档版本历史

- **v1.1.0** (2026-03-23):
  - 修正 API Secret 存储方式（使用 AES-256 加密而非 bcrypt 哈希）
  - 新增管理员表和管理后台认证设计
  - 新增租户配额表和审计日志表
  - 新增任务取消、重试、超时处理等业务流程
  - 补充完整的配置管理 API（TiDB/S3/模板）
  - 补充任务取消、批量查询、文件下载代理等 API
  - 新增健康检查和就绪检查 API
  - 优化部署模式说明，明确单进程和多进程部署
  - 新增配置服务和配置热更新机制
  - 更新实施计划，从 7 周调整为 10 周（增加技术验证阶段）
  - 补充人员配置和验收标准
  - 完善安全设计，增加请求签名、CSRF 防护、RBAC 权限控制

- **v1.0.0** (2026-03-23): 初始版本

---

**文档版本：** 1.1.0
**最后更新：** 2026-03-23
