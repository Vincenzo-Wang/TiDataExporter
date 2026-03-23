# Claw Export Platform - API 接口文档

> 版本: v1.0.0  
> 基础地址: `http://your-domain:8080`  
> 最后更新: 2026-03-23

---

## 目录

1. [概述](#概述)
2. [认证方式](#认证方式)
3. [通用说明](#通用说明)
4. [开放 API](#开放-api)
5. [管理 API](#管理-api)
6. [错误码说明](#错误码说明)
7. [更新日志](#更新日志)

---

## 概述

Claw Export Platform 提供两套 API：

| API 类型 | 认证方式 | 用途 | 前缀 |
|----------|----------|------|------|
| **开放 API** | API Key + Secret | 租户调用导出服务 | `/api/v1/export` |
| **管理 API** | JWT Token | 管理后台操作 | `/api/v1/admin` |

---

## 认证方式

### API Key/Secret 认证（开放 API）

在请求头中携带：

```
X-API-Key: your-api-key
X-API-Secret: your-api-secret
```

> 获取方式：登录管理后台，在租户详情中查看或重置

### JWT Token 认证（管理 API）

在请求头中携带：

```
Authorization: Bearer <your-jwt-token>
```

> Token 有效期 24 小时，可通过刷新接口续期

---

## 通用说明

### 请求格式

```http
Content-Type: application/json
```

### 响应格式

#### 成功响应

```json
{
  "code": 0,
  "message": "success",
  "data": { ... }
}
```

#### 错误响应

```json
{
  "code": 40001,
  "message": "参数错误",
  "data": null
}
```

### 分页参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| page | int | 1 | 页码 |
| page_size | int | 20 | 每页数量（最大 100） |

### 分页响应

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 100,
    "page": 1,
    "page_size": 20,
    "items": [...]
  }
}
```

---

## 开放 API

### 1. 创建导出任务

创建一个新的数据导出任务。

**请求**

```http
POST /api/v1/export/tasks
X-API-Key: your-api-key
X-API-Secret: your-api-secret
Content-Type: application/json
```

**请求体**

```json
{
  "tidb_config_name": "production-db",
  "s3_config_name": "backup-bucket",
  "sql_text": "SELECT * FROM users WHERE created_at > '2024-01-01'",
  "filetype": "sql",
  "compress": "gzip",
  "retention_hours": 168,
  "task_name": "用户数据导出",
  "priority": 5
}
```

**参数说明**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| tidb_config_name | string | 是 | TiDB 配置名称（需提前创建） |
| s3_config_name | string | 是 | S3 配置名称（需提前创建） |
| sql_text | string | 是 | SQL 查询语句 |
| filetype | string | 否 | 文件类型：`sql`（默认）、`csv` |
| compress | string | 否 | 压缩方式：`gzip`、`snappy`、`zstd`、空（不压缩） |
| retention_hours | int | 否 | 文件保留时间（小时），默认 168（7天） |
| task_name | string | 否 | 任务名称 |
| priority | int | 否 | 优先级 1-10，默认 5，越大越优先 |

**SQL 安全限制**

以下 SQL 关键字被禁止：
- `INSERT`、`UPDATE`、`DELETE`、`DROP`、`ALTER`、`CREATE`、`TRUNCATE`
- `GRANT`、`REVOKE`
- `INTO OUTFILE`、`LOAD DATA`

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": 123456789,
    "status": "pending",
    "created_at": "2026-03-23T10:00:00Z"
  }
}
```

**状态码**

| HTTP 状态码 | 说明 |
|-------------|------|
| 202 | 任务创建成功，等待执行 |
| 400 | 参数错误 |
| 401 | 认证失败 |
| 403 | 配额超限 |

---

### 2. 查询任务状态

查询指定任务的执行状态。

**请求**

```http
GET /api/v1/export/tasks/{task_id}
X-API-Key: your-api-key
X-API-Secret: your-api-secret
```

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| task_id | int | 任务 ID |

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": 123456789,
    "task_name": "用户数据导出",
    "status": "success",
    "file_url": "https://s3.amazonaws.com/bucket/path/to/file.sql.gz",
    "file_size": 1048576,
    "row_count": 10000,
    "started_at": "2026-03-23T10:01:00Z",
    "completed_at": "2026-03-23T10:05:00Z",
    "expires_at": "2026-03-30T10:05:00Z"
  }
}
```

**任务状态说明**

| 状态 | 说明 |
|------|------|
| pending | 等待执行 |
| running | 执行中 |
| success | 执行成功 |
| failed | 执行失败 |
| canceled | 已取消 |
| expired | 文件已过期 |

---

### 3. 取消任务

取消正在执行或等待中的任务。

**请求**

```http
DELETE /api/v1/export/tasks/{task_id}
X-API-Key: your-api-key
X-API-Secret: your-api-secret
```

**响应**

```json
{
  "code": 0,
  "message": "任务已取消",
  "data": {
    "task_id": 123456789,
    "status": "canceled"
  }
}
```

**注意**

- 已完成、已失败、已取消的任务无法再次取消
- 运行中的任务取消可能需要几秒钟时间

---

### 4. 批量查询任务

批量查询多个任务的状态。

**请求**

```http
POST /api/v1/export/tasks/batch
X-API-Key: your-api-key
X-API-Secret: your-api-secret
Content-Type: application/json
```

**请求体**

```json
{
  "task_ids": [123456789, 123456790, 123456791]
}
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "task_id": 123456789,
      "status": "success",
      "file_url": "https://..."
    },
    {
      "task_id": 123456790,
      "status": "running"
    },
    {
      "task_id": 123456791,
      "status": "failed",
      "error_message": "Connection timeout"
    }
  ]
}
```

---

### 5. 获取文件下载链接

获取任务导出文件的下载链接。

**请求**

```http
GET /api/v1/export/files/{task_id}
X-API-Key: your-api-key
X-API-Secret: your-api-secret
```

**响应**

返回 HTTP 302 重定向到 S3 文件地址。

**错误情况**

| 错误 | 说明 |
|------|------|
| 任务不存在 | 404 |
| 文件不存在 | 任务未完成或失败 |
| 文件已过期 | 超过保留时间 |

---

## 管理 API

### 认证接口

#### 1. 管理员登录

**请求**

```http
POST /api/v1/admin/auth/login
Content-Type: application/json
```

**请求体**

```json
{
  "username": "admin",
  "password": "admin123"
}
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 86400,
    "user": {
      "id": 1,
      "username": "admin",
      "role": "admin"
    }
  }
}
```

---

#### 2. 刷新 Token

**请求**

```http
POST /api/v1/admin/auth/refresh
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 86400
  }
}
```

---

#### 3. 登出

**请求**

```http
POST /api/v1/admin/auth/logout
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "登出成功",
  "data": null
}
```

---

#### 4. 获取当前用户信息

**请求**

```http
GET /api/v1/admin/auth/profile
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com",
    "role": "admin",
    "last_login_at": "2026-03-23T09:00:00Z",
    "created_at": "2026-03-01T00:00:00Z"
  }
}
```

---

#### 5. 修改密码

**请求**

```http
PUT /api/v1/admin/auth/password
Authorization: Bearer <token>
Content-Type: application/json
```

**请求体**

```json
{
  "old_password": "admin123",
  "new_password": "newSecurePassword123"
}
```

**响应**

```json
{
  "code": 0,
  "message": "密码修改成功",
  "data": null
}
```

---

### 租户管理接口

#### 1. 获取租户列表

**请求**

```http
GET /api/v1/admin/tenants?page=1&page_size=20
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 100,
    "page": 1,
    "page_size": 20,
    "items": [
      {
        "id": 1,
        "name": "租户A",
        "api_key": "ak_xxxxxxxxxxxxxxxx",
        "status": 1,
        "created_at": "2026-03-01T00:00:00Z"
      }
    ]
  }
}
```

---

#### 2. 创建租户

**请求**

```http
POST /api/v1/admin/tenants
Authorization: Bearer <token>
Content-Type: application/json
```

**请求体**

```json
{
  "name": "新租户",
  "contact_email": "contact@example.com"
}
```

**响应**

```json
{
  "code": 0,
  "message": "创建成功",
  "data": {
    "id": 2,
    "name": "新租户",
    "api_key": "ak_xxxxxxxxxxxxxxxx",
    "api_secret": "as_xxxxxxxxxxxxxxxx",
    "status": 1
  }
}
```

> **重要**: `api_secret` 仅在创建时返回一次，请妥善保存！

---

#### 3. 获取租户详情

**请求**

```http
GET /api/v1/admin/tenants/{id}
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1,
    "name": "租户A",
    "api_key": "ak_xxxxxxxxxxxxxxxx",
    "status": 1,
    "quota": {
      "max_concurrent_tasks": 5,
      "max_daily_tasks": 100,
      "max_daily_size_gb": 50
    },
    "created_at": "2026-03-01T00:00:00Z",
    "updated_at": "2026-03-23T10:00:00Z"
  }
}
```

---

#### 4. 更新租户

**请求**

```http
PUT /api/v1/admin/tenants/{id}
Authorization: Bearer <token>
Content-Type: application/json
```

**请求体**

```json
{
  "name": "更新后的租户名",
  "status": 1
}
```

---

#### 5. 删除租户

**请求**

```http
DELETE /api/v1/admin/tenants/{id}
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "删除成功",
  "data": null
}
```

> 删除租户会同时删除该租户的所有配置和历史任务记录

---

#### 6. 重置租户密钥

**请求**

```http
POST /api/v1/admin/tenants/{id}/regenerate-keys
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "密钥重置成功",
  "data": {
    "api_key": "ak_newxxxxxxxxxxxx",
    "api_secret": "as_newxxxxxxxxxxxx"
  }
}
```

> **重要**: 重置后旧密钥立即失效，`api_secret` 仅返回一次！

---

### 任务管理接口

#### 1. 获取任务列表

**请求**

```http
GET /api/v1/admin/tasks?page=1&page_size=20&status=success&tenant_id=1
Authorization: Bearer <token>
```

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码 |
| page_size | int | 每页数量 |
| status | string | 任务状态筛选 |
| tenant_id | int | 租户 ID 筛选 |
| start_date | string | 开始日期（YYYY-MM-DD） |
| end_date | string | 结束日期（YYYY-MM-DD） |

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 1000,
    "page": 1,
    "page_size": 20,
    "items": [
      {
        "id": 123456789,
        "task_name": "用户数据导出",
        "tenant_id": 1,
        "status": "success",
        "file_size": 1048576,
        "row_count": 10000,
        "created_at": "2026-03-23T10:00:00Z",
        "completed_at": "2026-03-23T10:05:00Z"
      }
    ]
  }
}
```

---

#### 2. 获取任务详情

**请求**

```http
GET /api/v1/admin/tasks/{id}
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 123456789,
    "task_name": "用户数据导出",
    "tenant_id": 1,
    "sql_text": "SELECT * FROM users WHERE ...",
    "filetype": "sql",
    "compress": "gzip",
    "status": "success",
    "file_url": "https://s3.amazonaws.com/...",
    "file_size": 1048576,
    "row_count": 10000,
    "error_message": "",
    "retry_count": 0,
    "priority": 5,
    "retention_hours": 168,
    "created_at": "2026-03-23T10:00:00Z",
    "started_at": "2026-03-23T10:01:00Z",
    "completed_at": "2026-03-23T10:05:00Z",
    "expires_at": "2026-03-30T10:05:00Z"
  }
}
```

---

#### 3. 获取任务日志

**请求**

```http
GET /api/v1/admin/tasks/{id}/logs
Authorization: Bearer <token>
```

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "log_level": "INFO",
      "message": "Task started",
      "created_at": "2026-03-23T10:01:00Z"
    },
    {
      "log_level": "INFO",
      "message": "Exported 5000 rows",
      "created_at": "2026-03-23T10:03:00Z"
    },
    {
      "log_level": "INFO",
      "message": "Task completed",
      "created_at": "2026-03-23T10:05:00Z"
    }
  ]
}
```

---

### 配置管理接口

#### TiDB 配置

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/admin/tidb-configs` | GET | 获取配置列表 |
| `/api/v1/admin/tidb-configs` | POST | 创建配置 |
| `/api/v1/admin/tidb-configs/{id}` | PUT | 更新配置 |
| `/api/v1/admin/tidb-configs/{id}` | DELETE | 删除配置 |

**创建 TiDB 配置请求体**

```json
{
  "name": "production-db",
  "host": "tidb.example.com",
  "port": 4000,
  "username": "exporter",
  "password": "password123",
  "database": "mydb",
  "is_default": true
}
```

---

#### S3 配置

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/admin/s3-configs` | GET | 获取配置列表 |
| `/api/v1/admin/s3-configs` | POST | 创建配置 |
| `/api/v1/admin/s3-configs/{id}` | PUT | 更新配置 |
| `/api/v1/admin/s3-configs/{id}` | DELETE | 删除配置 |

**创建 S3 配置请求体**

```json
{
  "name": "backup-bucket",
  "endpoint": "https://s3.amazonaws.com",
  "access_key": "AKIAIOSFODNN7EXAMPLE",
  "secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
  "bucket": "my-backup-bucket",
  "region": "us-east-1",
  "path_prefix": "exports/",
  "is_default": true
}
```

---

### 审计日志

**请求**

```http
GET /api/v1/admin/audit-logs?page=1&page_size=20&action=task.create
Authorization: Bearer <token>
```

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码 |
| page_size | int | 每页数量 |
| action | string | 操作类型筛选 |
| tenant_id | int | 租户 ID 筛选 |
| admin_id | int | 管理员 ID 筛选 |
| start_date | string | 开始日期 |
| end_date | string | 结束日期 |

**响应**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 500,
    "items": [
      {
        "id": 1,
        "tenant_id": 1,
        "admin_id": 1,
        "action": "task.create",
        "resource_type": "task",
        "resource_id": 123456789,
        "request_ip": "192.168.1.100",
        "result": "success",
        "created_at": "2026-03-23T10:00:00Z"
      }
    ]
  }
}
```

---

## 错误码说明

### 通用错误码

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 40000 | 未知错误 |
| 40001 | 参数错误 |
| 40002 | 资源不存在 |
| 40003 | 资源已存在 |

### 认证错误码

| 错误码 | 说明 |
|--------|------|
| 40100 | 未授权（缺少认证信息） |
| 40101 | 认证失败（用户名或密码错误） |
| 40102 | Token 无效或已过期 |
| 40103 | API Key 无效 |
| 40104 | API Secret 错误 |

### 权限错误码

| 错误码 | 说明 |
|--------|------|
| 40300 | 无权限 |
| 40301 | 账号已被禁用 |
| 40302 | 租户已被禁用 |

### 配额错误码

| 错误码 | 说明 |
|--------|------|
| 42900 | 请求频率超限 |
| 42901 | 并发任务数已达上限 |
| 42902 | 每日任务数已达上限 |
| 42903 | 每日导出量已达上限 |

### 业务错误码

| 错误码 | 说明 |
|--------|------|
| 50000 | 服务器内部错误 |
| 50001 | 数据库错误 |
| 50002 | Redis 错误 |
| 50003 | S3 上传失败 |
| 50004 | Dumpling 执行失败 |
| 50005 | SQL 语法错误 |

---

## 更新日志

### v1.0.0 (2026-03-23)
- 初始版本发布
- 开放 API：任务创建、查询、取消、批量查询、文件下载
- 管理 API：认证、租户管理、任务管理、配置管理、审计日志

---

## 联系支持

- GitHub Issues: [项目地址]/issues
- 邮件: support@example.com

---

*最后更新: 2026-03-23*
