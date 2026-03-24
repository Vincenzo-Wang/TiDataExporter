# 导出任务 API 使用指南

本文档详细说明如何通过 API 创建导出任务、获取结果，以及系统内部处理流程。

---

## 目录

1. [认证方式](#认证方式)
2. [创建导出任务](#创建导出任务)
3. [查询任务状态](#查询任务状态)
4. [下载导出文件](#下载导出文件)
5. [取消任务](#取消任务)
6. [完整使用示例](#完整使用示例)
7. [任务处理流程](#任务处理流程)
8. [关键代码节点](#关键代码节点)

---

## 认证方式

租户 API 使用 **API Key + API Secret** 认证。

### 请求头

```
X-API-Key: sk_live_xxxxxxxxxxxxxxxx
X-API-Secret: sk_secret_xxxxxxxxxxxxxxxx
X-Timestamp: 1700000000
X-Signature: <签名值>
```

### 签名算法

```
签名字符串 = HTTP方法 + "\n" + 请求路径 + "\n" + 时间戳 + "\n" + 请求体MD5
签名值 = HMAC-SHA256(API_Secret, 签名字符串)
```

---

## 创建导出任务

### 请求

```http
POST /api/v1/export/tasks
Content-Type: application/json
X-API-Key: sk_live_xxxxx
X-API-Secret: sk_secret_xxxxx
X-Timestamp: 1700000000
X-Signature: xxxxx

{
    "tidb_config_name": "production-tidb",
    "s3_config_name": "default-s3",
    "sql_text": "SELECT id, name, email, created_at FROM users WHERE created_at >= '2024-01-01'",
    "filetype": "csv",
    "compress": "gz",
    "retention_hours": 168,
    "task_name": "用户数据导出-2024Q1",
    "priority": 5
}
```

### 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `tidb_config_name` | string | ✅ | TiDB 配置名称（需预先配置） |
| `s3_config_name` | string | ✅ | S3 存储配置名称（需预先配置） |
| `sql_text` | string | ✅ | 导出的 SQL 语句（仅支持 SELECT） |
| `filetype` | string | ❌ | 输出格式：`sql`（默认）或 `csv` |
| `compress` | string | ❌ | 压缩格式：`gz`、`zstd` 等，不填则不压缩 |
| `retention_hours` | int | ❌ | 文件保留时间（小时），默认 168（7天） |
| `task_name` | string | ❌ | 任务名称，便于识别 |
| `priority` | int | ❌ | 优先级 1-10，数字越大越优先，默认 5 |

### 响应

```json
{
    "code": 0,
    "message": "accepted",
    "data": {
        "task_id": 12345,
        "status": "pending",
        "created_at": "2024-01-15T10:30:00Z"
    }
}
```

### 关键代码位置

```
backend/api/v1/handlers.go:53-150   CreateTask 函数
```

**处理流程：**

1. 验证参数（第 56-60 行）
2. SQL 安全检查（第 62-69 行）→ `services/export/executor.go:191-209`
3. 查询 TiDB/S3 配置（第 82-94 行）
4. 配额检查（第 96-115 行）
5. 创建任务记录（第 117-136 行）
6. 任务入队（第 138-143 行）→ `pkg/queue/queue.go:42-79`

---

## 查询任务状态

### 请求

```http
GET /api/v1/export/tasks/{task_id}
X-API-Key: sk_live_xxxxx
X-API-Secret: sk_secret_xxxxx
X-Timestamp: 1700000000
X-Signature: xxxxx
```

### 响应

```json
{
    "code": 0,
    "message": "success",
    "data": {
        "task_id": 12345,
        "task_name": "用户数据导出-2024Q1",
        "status": "success",
        "file_url": "exports/12345/output.csv.gz",
        "file_size": 1048576,
        "row_count": 50000,
        "started_at": "2024-01-15T10:30:05Z",
        "completed_at": "2024-01-15T10:32:30Z",
        "expires_at": "2024-01-22T10:32:30Z"
    }
}
```

### 任务状态说明

| 状态 | 说明 |
|------|------|
| `pending` | 等待处理，在队列中排队 |
| `running` | 正在执行导出 |
| `success` | 导出成功，文件可下载 |
| `failed` | 导出失败，查看 `error_message` |
| `canceled` | 已取消 |
| `expired` | 文件已过期 |

### 关键代码位置

```
backend/api/v1/handlers.go:152-184   GetTask 函数
```

---

## 下载导出文件

### 请求

```http
GET /api/v1/export/files/{task_id}
X-API-Key: sk_live_xxxxx
X-API-Secret: sk_secret_xxxxx
X-Timestamp: 1700000000
X-Signature: xxxxx
```

### 响应

返回 **HTTP 302 重定向** 到预签名 S3 URL：

```
Location: https://s3.example.com/bucket/exports/12345/output.csv.gz?X-Amz-Signature=xxxxx&X-Amz-Expires=3600
```

> **安全说明**：系统使用预签名 URL，不会直接暴露 S3 凭证。URL 有效期与文件剩余有效期一致（至少 1 小时）。

### 关键代码位置

```
backend/api/v1/handlers.go:256-330   GetFile 函数
```

**处理流程：**

1. 验证任务归属（第 262-265 行）
2. 检查文件是否存在和是否过期（第 267-276 行）
3. 获取 S3 配置并解密密钥（第 278-292 行）
4. 创建 S3 客户端（第 294-307 行）
5. 生成预签名 URL（第 309-326 行）
6. 重定向到预签名 URL（第 328 行）

---

## 取消任务

### 请求

```http
DELETE /api/v1/export/tasks/{task_id}
X-API-Key: sk_live_xxxxx
X-API-Secret: sk_secret_xxxxx
X-Timestamp: 1700000000
X-Signature: xxxxx
```

### 响应

```json
{
    "code": 0,
    "message": "任务已取消",
    "data": {
        "task_id": 12345,
        "status": "canceled"
    }
}
```

### 关键代码位置

```
backend/api/v1/handlers.go:186-218   CancelTask 函数
```

---

## 完整使用示例

### Python 示例

```python
import requests
import hashlib
import hmac
import time
import json

class ExportClient:
    def __init__(self, base_url, api_key, api_secret):
        self.base_url = base_url
        self.api_key = api_key
        self.api_secret = api_secret
    
    def _sign(self, method, path, timestamp, body=""):
        body_md5 = hashlib.md5(body.encode()).hexdigest() if body else ""
        sign_str = f"{method}\n{path}\n{timestamp}\n{body_md5}"
        signature = hmac.new(
            self.api_secret.encode(),
            sign_str.encode(),
            hashlib.sha256
        ).hexdigest()
        return signature
    
    def _headers(self, method, path, body=""):
        timestamp = str(int(time.time()))
        return {
            "Content-Type": "application/json",
            "X-API-Key": self.api_key,
            "X-API-Secret": self.api_secret,
            "X-Timestamp": timestamp,
            "X-Signature": self._sign(method, path, timestamp, body)
        }
    
    def create_task(self, **params):
        path = "/api/v1/export/tasks"
        body = json.dumps(params)
        resp = requests.post(
            f"{self.base_url}{path}",
            headers=self._headers("POST", path, body),
            data=body
        )
        return resp.json()
    
    def get_task(self, task_id):
        path = f"/api/v1/export/tasks/{task_id}"
        resp = requests.get(
            f"{self.base_url}{path}",
            headers=self._headers("GET", path)
        )
        return resp.json()
    
    def download_file(self, task_id, output_path):
        path = f"/api/v1/export/files/{task_id}"
        resp = requests.get(
            f"{self.base_url}{path}",
            headers=self._headers("GET", path),
            allow_redirects=True
        )
        with open(output_path, "wb") as f:
            f.write(resp.content)
        return output_path

# 使用示例
client = ExportClient(
    base_url="https://export.example.com",
    api_key="sk_live_xxxxx",
    api_secret="sk_secret_xxxxx"
)

# 1. 创建任务
result = client.create_task(
    tidb_config_name="production-tidb",
    s3_config_name="default-s3",
    sql_text="SELECT * FROM users WHERE created_at >= '2024-01-01'",
    filetype="csv",
    compress="gz",
    task_name="用户导出"
)
task_id = result["data"]["task_id"]
print(f"任务已创建: {task_id}")

# 2. 轮询状态
import time
while True:
    task = client.get_task(task_id)
    status = task["data"]["status"]
    print(f"任务状态: {status}")
    
    if status in ["success", "failed", "canceled"]:
        break
    time.sleep(5)

# 3. 下载文件
if task["data"]["status"] == "success":
    client.download_file(task_id, "output.csv.gz")
    print("文件下载完成")
```

---

## 任务处理流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              完整任务处理流程                                  │
└─────────────────────────────────────────────────────────────────────────────┘

用户请求                API服务                 数据库              队列              Worker
   │                      │                      │                  │                  │
   │  POST /tasks         │                      │                  │                  │
   ├─────────────────────>│                      │                  │                  │
   │                      │  1. 验证API Key      │                  │                  │
   │                      ├─────────────────────>│                  │                  │
   │                      │                      │                  │                  │
   │                      │  2. SQL安全检查      │                  │                  │
   │                      ├──────────┐           │                  │                  │
   │                      │          │           │                  │                  │
   │                      │  3. 配额检查         │                  │                  │
   │                      ├─────────────────────>│                  │                  │
   │                      │                      │                  │                  │
   │                      │  4. 创建任务记录     │                  │                  │
   │                      ├─────────────────────>│                  │                  │
   │                      │      task (pending)  │                  │                  │
   │                      │                      │                  │                  │
   │                      │  5. 任务入队         │                  │                  │
   │                      ├─────────────────────────────────────────>│                  │
   │                      │                      │                  │  Redis Stream    │
   │<─────────────────────┤  202 Accepted        │                  │                  │
   │   {"task_id": 123}   │                      │                  │                  │
   │                      │                      │                  │                  │
   │                      │                      │                  │  6. 拉取任务     │
   │                      │                      │                  │<─────────────────┤
   │                      │                      │                  │                  │
   │                      │                      │  7. 获取任务详情 │                  │
   │                      │                      │<─────────────────────────────────────┤
   │                      │                      │                  │                  │
   │                      │                      │  8. 更新状态     │                  │
   │                      │                      │<─────────────────────────────────────┤
   │                      │                      │  task (running)  │                  │
   │                      │                      │                  │                  │
   │                      │                      │                  │     9. 执行导出   │
   │                      │                      │                  │     Dumpling     │
   │                      │                      │                  │     执行SQL      │
   │                      │                      │                  │     生成文件     │
   │                      │                      │                  │                  │
   │                      │                      │                  │     10. 上传S3   │
   │                      │                      │                  │                  │
   │                      │                      │  11. 更新结果    │                  │
   │                      │                      │<─────────────────────────────────────┤
   │                      │                      │  task (success)  │                  │
   │                      │                      │  file_url        │                  │
   │                      │                      │                  │                  │
   │  GET /tasks/123      │                      │                  │                  │
   ├─────────────────────>│                      │                  │                  │
   │                      │  查询任务状态        │                  │                  │
   │                      ├─────────────────────>│                  │                  │
   │<─────────────────────┤  {"status":"success"}│                  │                  │
   │                      │                      │                  │                  │
   │  GET /files/123      │                      │                  │                  │
   ├─────────────────────>│                      │                  │                  │
   │                      │  生成预签名URL       │                  │                  │
   │<─────────────────────┤  302 Redirect        │                  │                  │
   │   Location: S3 URL   │                      │                  │                  │
   │                      │                      │                  │                  │
   ▼                      ▼                      ▼                  ▼                  ▼
```

---

## 关键代码节点

### 1. 任务创建入口

**文件**: `backend/api/v1/handlers.go`

```go
// 第53-150行: CreateTask 函数
func (h *ExportHandler) CreateTask(c *gin.Context) {
    // 1. 验证请求参数
    var req CreateTaskRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.BadRequest(c, "参数验证失败: "+err.Error())
        return
    }

    // 2. SQL 安全检查
    if err := export.ValidateSQL(req.SqlText); err != nil {
        utils.BadRequestWithData(c, "SQL语句包含非法关键字", ...)
        return
    }

    // 3. 配额检查
    // 4. 创建任务记录
    task := &models.ExportTask{...}
    h.db.Create(task)

    // 5. 加入队列
    h.queue.Enqueue(c.Request.Context(), task)
}
```

### 2. SQL 安全验证

**文件**: `backend/services/export/executor.go`

```go
// 第191-209行: ValidateSQL 函数
func ValidateSQL(sqlText string) error {
    // 禁止的关键字：DROP, DELETE, TRUNCATE, ALTER, CREATE, INSERT, UPDATE 等
    dangerousKeywords := []string{
        "DROP", "DELETE", "TRUNCATE", "ALTER", "CREATE", "INSERT", "UPDATE",
        "GRANT", "REVOKE", "EXEC", "EXECUTE", "CALL",
    }
    // 检查是否包含危险关键字
    for _, keyword := range dangerousKeywords {
        if strings.Contains(sqlUpper, keyword+" ") {
            return fmt.Errorf("SQL contains forbidden keyword: %s", keyword)
        }
    }
    return nil
}
```

### 3. 任务队列入队

**文件**: `backend/pkg/queue/queue.go`

```go
// 第42-79行: Enqueue 函数
func (q *Queue) Enqueue(ctx context.Context, task *models.ExportTask) error {
    msg := TaskMessage{
        TaskID:       task.ID,
        TenantID:     task.TenantID,
        SqlText:      task.SqlText,
        Filetype:     task.Filetype,
        Compress:     task.Compress,
        TiDBConfigID: task.TiDBConfigID,
        S3ConfigID:   task.S3ConfigID,
    }
    data, _ := json.Marshal(msg)
    
    // 写入 Redis Stream
    fields := map[string]interface{}{
        "task_id":    task.ID,
        "data":       string(data),
        "priority":   task.Priority,
    }
    q.redis.AddMessage(ctx, fields)
}
```

### 4. Worker 任务处理

**文件**: `backend/workers/worker.go`

```go
// 第100-133行: processTask 函数
func (w *Worker) processTask() {
    // 1. 从队列获取任务
    taskMsg, msgID, _ := w.queue.Dequeue(ctx)
    
    // 2. 处理任务
    w.handleTask(ctx, taskMsg)
    
    // 3. 确认消息
    w.queue.Ack(ctx, msgID)
}

// 第135-200行: handleTask 函数
func (w *Worker) handleTask(ctx context.Context, msg *queue.TaskMessage) error {
    // 1. 获取任务详情
    w.db.First(&task, msg.TaskID)
    
    // 2. 更新状态为 running
    w.db.Model(&task).Updates(map[string]interface{}{
        "status":     models.TaskStatusRunning,
        "started_at": time.Now(),
    })
    
    // 3. 执行导出
    executor := export.NewExecutor(w.db, s3Client, w.encryptor, w.workDir, w.logger)
    result, _ := executor.Execute(ctx, task.ID, &tidbConfig, msg.SqlText, ...)
    
    // 4. 更新成功状态
    w.db.Model(&task).Updates(map[string]interface{}{
        "status":       models.TaskStatusSuccess,
        "file_url":     result.FileURL,
        "file_size":    result.FileSize,
    })
}
```

### 5. 导出执行器

**文件**: `backend/services/export/executor.go`

```go
// 第40-118行: Execute 函数
func (e *Executor) Execute(ctx context.Context, taskID int64, tidbConfig *models.TiDBConfig, ...) (*ExecutionResult, error) {
    // 1. 解密数据库密码
    password, _ := e.encryptor.Decrypt(tidbConfig.PasswordEncrypted)
    
    // 2. 创建工作目录
    taskDir := filepath.Join(e.workDir, fmt.Sprintf("task_%d", taskID))
    os.MkdirAll(taskDir, 0755)
    
    // 3. 构建 Dumpling 命令
    cmd := e.buildDumplingCommand(tidbConfig, password, sqlText, ...)
    
    // 4. 执行命令
    output, err := cmd.CombinedOutput()
    
    // 5. 上传到 S3
    e.s3Client.Upload(ctx, s3Key, file, fileSize, contentType)
    
    return &ExecutionResult{FileURL: s3Key, FileSize: fileSize}, nil
}

// 第127-166行: buildDumplingCommand 函数
func (e *Executor) buildDumplingCommand(...) *exec.Cmd {
    args := []string{
        fmt.Sprintf("--host=%s", tidbConfig.Host),
        fmt.Sprintf("--port=%d", tidbConfig.Port),
        fmt.Sprintf("--user=%s", tidbConfig.Username),
        fmt.Sprintf("--sql=%s", sqlText),
        "--threads=4",
        "--rows=100000",
    }
    return exec.Command(dumplingPath, args...)
}
```

### 6. 文件下载（预签名 URL）

**文件**: `backend/api/v1/handlers.go`

```go
// 第256-330行: GetFile 函数
func (h *ExportHandler) GetFile(c *gin.Context) {
    // 1. 验证任务归属
    h.db.Where("id = ? AND tenant_id = ?", taskID, tenantID).First(&task)
    
    // 2. 检查文件是否过期
    if task.ExpiresAt.Before(time.Now()) {
        utils.NotFound(c, "文件已过期")
        return
    }
    
    // 3. 获取 S3 配置
    h.db.First(&s3Config, task.S3ConfigID)
    
    // 4. 解密 SecretKey
    secretKey, _ := h.encryptor.Decrypt(s3Config.SecretKeyEncrypted)
    
    // 5. 创建 S3 客户端
    s3Client, _ := s3.NewClient(ctx, s3.Config{...})
    
    // 6. 生成预签名 URL
    presignedURL, _ := s3Client.GetPresignedURL(ctx, task.FileURL, expiresIn)
    
    // 7. 重定向
    c.Redirect(http.StatusFound, presignedURL)
}
```

### 7. S3 预签名 URL 生成

**文件**: `backend/services/s3/s3.go`

```go
// GetPresignedURL 生成预签名下载URL
func (c *Client) GetPresignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error) {
    req, _ := c.client.PresignGetObject(ctx, &s3.GetObjectInput{
        Bucket: aws.String(c.bucket),
        Key:    aws.String(c.pathPrefix + key),
    }, minio.PutObjectOptions{Expires: expiresIn})
    return req.URL(), nil
}
```

---

## 文件结构总览

```
backend/
├── api/
│   ├── router.go          # 路由配置
│   ├── middleware/        # 认证中间件
│   │   └── auth.go        # API Key / JWT 认证
│   └── v1/
│       └── handlers.go    # API 处理器（入口）
│
├── pkg/
│   ├── queue/
│   │   └── queue.go       # Redis Stream 任务队列
│   └── encryption/
│       └── encryptor.go   # 敏感数据加密
│
├── services/
│   ├── export/
│   │   └── executor.go    # Dumpling 执行器
│   ├── s3/
│   │   └── s3.go          # S3 客户端封装
│   └── task/
│       └── manager.go     # 任务管理（超时检查）
│
├── workers/
│   └── worker.go          # Worker 池，任务执行
│
└── models/
    └── task.go            # 数据模型定义
```

---

## 错误码说明

| HTTP 状态码 | code | 说明 |
|-------------|------|------|
| 200 | 0 | 成功 |
| 202 | 0 | 已接受（任务创建） |
| 400 | 400001 | 参数验证失败 |
| 400 | 400002 | SQL 包含非法关键字 |
| 401 | 401001 | 认证失败 |
| 403 | 403001 | 配额超限 |
| 404 | 404001 | 资源不存在 |
| 404 | 404002 | 文件已过期 |
| 500 | 500001 | 内部错误 |
