# Claw Export Platform - 开发部署文档

## 目录

1. [环境准备](#环境准备)
2. [配置说明](#配置说明)
3. [部署方式](#部署方式)
4. [前后端分离部署](#前后端分离部署)
5. [运维指南](#运维指南)

---

## 环境准备

### 系统要求

| 组件 | 最低配置 | 推荐配置 |
|------|----------|----------|
| CPU | 2 核 | 4 核+ |
| 内存 | 4 GB | 8 GB+ |
| 磁盘 | 20 GB | 100 GB+ SSD |
| 操作系统 | Linux/macOS/Windows | Ubuntu 22.04 / CentOS 8+ |

### 软件依赖

#### Docker 部署（推荐）

- Docker 20.10+
- Docker Compose 1.18.0+（兼容旧版本）

#### 开发环境

- Go 1.25+
- Node.js 18+
- MySQL 8.0+
- Redis 7+
- Dumpling（TiUP 安装）

---

## 配置说明

### 环境变量配置 (.env)

```bash
# ============================================
# 应用配置
# ============================================
SERVER_MODE=release             # 可选：debug / release
SERVER_PORT=8080                # 后端服务端口
SERVER_TIMEOUT=30s              # HTTP 读写超时
SERVER_SHUTDOWN_TIMEOUT=10s     # 优雅停机超时

# ============================================
# 数据库配置
# ============================================
MYSQL_ROOT_PASSWORD=root123     # MySQL root 密码（生产环境必须修改！）
MYSQL_DATABASE=claw_export      # 数据库名称
MYSQL_USER=claw                 # 数据库用户名
MYSQL_PASSWORD=claw123          # 数据库密码（生产环境必须修改！）
MYSQL_PORT=3306                 # MySQL 端口

# ============================================
# Redis 配置
# ============================================
REDIS_PASSWORD=redis123         # Redis 密码（生产环境必须修改！）
REDIS_PORT=6379                 # Redis 端口

# ============================================
# 安全配置（生产环境必须修改！）
# ============================================
# JWT 密钥：至少 32 个字符的随机字符串
JWT_SECRET=your-super-secret-jwt-key-change-in-production

# AES 加密密钥：必须是正好 32 字节的字符串
# 注意：长度必须正好 32 字节（用于 AES-256 加密）
# 生成方法：openssl rand -base64 32 | head -c 32
AES_KEY=0123456789abcdef0123456789abcdef

# ============================================
# 服务端口
# ============================================
FRONTEND_PORT=80                # 前端服务端口
BACKEND_PORT=8080               # 后端 API 端口

# ============================================
# Worker 配置
# ============================================
WORKER_COUNT=4                  # Worker 并发数

# ============================================
# S3 存储配置（可选，租户可自行配置）
# ============================================
# 支持多云存储：AWS S3、阿里云 OSS
# 配置方式：在前端管理界面创建 S3 配置时选择厂商
S3_ACCESS_KEY=                  # Access Key ID
S3_SECRET_KEY=                  # Secret Access Key
S3_ENDPOINT=                    # S3 Endpoint（AWS: https://s3.amazonaws.com，阿里云: oss-cn-hangzhou.aliyuncs.com）
S3_REGION=us-east-1             # S3 Region（阿里云可不填）
S3_BUCKET=                      # 默认 Bucket 名称

# ============================================
# Dumpling 配置
# ============================================
# Dumpling 可执行文件路径（宿主机绝对路径）
# 示例：/home/work/.tiup/components/dumpling/v8.5.5/dumpling
HOST_DUMPLING_PATH=/usr/local/bin/dumpling

# ============================================
# 日志配置
# ============================================
LOG_LEVEL=info                  # 日志级别：debug/info/warn/error
```

### 前端配置 (frontend/.env)

```bash
# API 地址（开发环境）
VITE_API_BASE_URL=http://localhost:8080

# 生产环境构建时，API 请求会通过 Nginx 反向代理
# 无需额外配置
```

---

## 部署方式

> 当前仓库保留两类研发部署方式：
> 1. 本地 MVP / 联调：使用 `docker-compose.yml` + `deploy.sh`
> 2. 测试环境直连云资源：使用 `.env.test` + `docker-compose.test.yml`

### 方式一：本地 Docker 一键部署（推荐用于开发/联调）

#### 自动初始化说明

项目支持数据库自动迁移，首次启动时会自动：

1. 创建所有数据库表
2. 插入默认管理员账号

默认管理员账号：
- 用户名：`admin`
- 密码：`admin123`

> ⚠️ **安全警告**：生产环境请务必修改默认密码！

#### 部署步骤

```bash
# 1. 克隆项目
git clone <repository-url>
cd claw-export-platform

# 2. 配置环境变量
cp .env.example .env
vim .env  # 修改必要的配置

# 3. 安装 Dumpling（如果尚未安装）
tiup install dumpling

# 4. 确认 Dumpling 路径并更新 .env
which dumpling  # 或 tiup list --installed
# 更新 .env 中的 HOST_DUMPLING_PATH

# 5. 执行部署脚本
chmod +x deploy.sh
./deploy.sh start

# 6. 查看服务状态
docker-compose ps

# 7. 查看日志（确认初始化成功）
docker-compose logs mysql | grep -A 10 "Database Init"

# 8. 验证登录
curl -X POST http://localhost:8080/api/v1/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

#### 服务架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     Claw Export Platform                        │
├─────────────────────────────────────────────────────────────────┤
│  claw-mysql      MySQL 8.0       数据存储         Port: 3306    │
│  claw-redis      Redis 7         缓存/队列        Port: 6379    │
│  claw-backend    Go Backend      API 服务         Port: 8080    │
│  claw-frontend   Nginx           前端静态文件      Port: 80      │
│  claw-worker     Go Worker       任务执行器       (内部通信)     │
└─────────────────────────────────────────────────────────────────┘
```

#### deploy.sh 命令说明

```bash
./deploy.sh start       # 启动所有服务（默认命令）
./deploy.sh stop        # 停止所有服务
./deploy.sh restart     # 重启所有服务
./deploy.sh status      # 查看服务状态
./deploy.sh logs        # 查看日志
./deploy.sh build       # 构建镜像
./deploy.sh migrate     # 执行数据库迁移
./deploy.sh clean       # 清理所有容器和卷
./deploy.sh backup      # 备份数据库
./deploy.sh help        # 显示帮助信息
```

#### 重新初始化数据库

如果需要重新初始化数据库（会删除所有数据）：

```bash
# 停止服务并删除数据卷
docker-compose down -v

# 重新启动（会自动执行初始化）
docker-compose up -d
```

### 方式二：测试环境 Docker Compose（直连阿里云 TiDB / Redis）

#### 适用场景

- 研发自行验证测试环境
- 只想保留一个 `.env.test` 和一个 `docker-compose.test.yml`
- 不在测试环境启动本地 `mysql` / `redis` 容器
- 直接连接阿里云 `TiDB` 与云 `Redis`

#### 需要修改的文件

- `.env.test`：填写测试环境云资源连接参数
- `docker-compose.test.yml`：启动 `frontend` / `backend` / `worker` / `migrate`
- `backend/Dockerfile.migrate`：构建迁移镜像

#### 推荐命令

```bash
# 1. 修改测试环境变量
vim .env.test

# 2. 先执行迁移
docker compose --env-file .env.test -f docker-compose.test.yml run --rm migrate

# 3. 启动测试环境服务
docker compose --env-file .env.test -f docker-compose.test.yml up -d --build backend worker frontend

# 4. 查看状态与日志
docker compose --env-file .env.test -f docker-compose.test.yml ps
docker compose --env-file .env.test -f docker-compose.test.yml logs -f backend worker frontend
```

#### TiDB 初始化与升级策略

- 所有初始化与升级 SQL 统一位于 `backend/migrations/up/*.sql`
- `000001_init_schema.up.sql` 现在就是最新完整基线，已包含 TiDB 兼容表结构与默认管理员 `admin / admin123`
- `migrate` 服务会调用 `backend/main.go` 中的迁移入口执行 SQL 文件
- 若你清理过历史迁移文件，请对新环境重新初始化数据库，不要复用旧的 `schema_migrations` 记录

### 方式三：本地 Docker 手动部署

```bash
# 1. 创建网络
docker network create claw-network

# 2. 启动 MySQL
docker-compose up -d mysql
# 等待 MySQL 就绪
sleep 30

# 3. 执行数据库迁移
./deploy.sh migrate

# 4. 启动 Redis
docker-compose up -d redis
# 等待 Redis 就绪
sleep 10

# 5. 启动后端
docker-compose up -d backend

# 6. 启动前端
docker-compose up -d frontend

# 7. 启动 Worker
docker-compose up -d worker

# 8. 检查服务
docker-compose ps
```

---

## 前后端分离部署

### 架构说明

```
┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐
│   Nginx/CDN     │      │   Backend API   │      │   MySQL/Redis   │
│   (前端静态文件)  │─────▶│   (Go Service)  │─────▶│   (数据存储)     │
│   Port: 80/443  │      │   Port: 8080    │      │   Internal      │
└─────────────────┘      └─────────────────┘      └─────────────────┘
```

### 前端独立部署

#### 1. 构建前端

```bash
cd frontend

# 安装依赖
npm install

# 构建生产版本
npm run build

# 构建产物在 dist/ 目录
```

#### 2. 部署到 Nginx

```nginx
# /etc/nginx/sites-available/claw-export.conf
server {
    listen 80;
    server_name your-domain.com;

    # 前端静态文件
    root /var/www/claw-export/dist;
    index index.html;

    # Gzip 压缩
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    # API 反向代理
    location /api {
        proxy_pass http://backend-server:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
    }

    # 前端路由
    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

#### 3. 部署到 CDN/OSS

```bash
# 上传到阿里云 OSS
ossutil cp -r dist/ oss://your-bucket/claw-export/

# 或上传到 AWS S3
aws s3 sync dist/ s3://your-bucket/claw-export/
```

### 后端独立部署

#### 1. 构建后端

```bash
cd backend

# 下载依赖
go mod download

# 构建 Linux 可执行文件
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o claw-export cmd/server/main.go

# 构建 Worker
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o claw-worker cmd/worker/main.go
```

#### 2. 说明

仓库当前只内置本地开发与测试环境直连云资源的 Compose 方案。
如需正式交付链路（如 `systemd`、`Jenkins`、宿主机 `Nginx`），请由运维团队按目标环境单独维护。

#### 3. Kubernetes 部署

```yaml
# k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: claw-export-backend
  namespace: claw-export
spec:
  replicas: 3
  selector:
    matchLabels:
      app: claw-export-backend
  template:
    metadata:
      labels:
        app: claw-export-backend
    spec:
      containers:
      - name: backend
        image: claw-export-backend:latest
        ports:
        - containerPort: 8080
        env:
        - name: SERVER_MODE
          value: "release"
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: claw-export-secrets
              key: db-host
        - name: AES_KEY
          valueFrom:
            secretKeyRef:
              name: claw-export-secrets
              key: aes-key
        # ... 其他环境变量
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: claw-export-worker
  namespace: claw-export
spec:
  replicas: 2
  selector:
    matchLabels:
      app: claw-export-worker
  template:
    metadata:
      labels:
        app: claw-export-worker
    spec:
      containers:
      - name: worker
        image: claw-export-worker:latest
        env:
        - name: WORKER_COUNT
          value: "4"
        - name: DUMPLING_PATH
          value: "/usr/local/bin/dumpling"
        # ... 其他环境变量
        volumeMounts:
        - name: dumpling
          mountPath: /usr/local/bin/dumpling
          readOnly: true
      volumes:
      - name: dumpling
        hostPath:
          path: /usr/local/bin/dumpling
          type: File
---
apiVersion: v1
kind: Service
metadata:
  name: claw-export-backend
  namespace: claw-export
spec:
  selector:
    app: claw-export-backend
  ports:
  - port: 8080
    targetPort: 8080
  type: ClusterIP
```

---

## 运维指南

### 数据库备份

```bash
# 手动备份
docker exec claw-mysql mysqldump -u root -p${MYSQL_ROOT_PASSWORD} claw_export > backup_$(date +%Y%m%d).sql

# 定时备份（crontab）
0 3 * * * /opt/claw-export/scripts/backup.sh
```

### 日志管理

```bash
# 查看后端日志
docker-compose logs -f backend --tail=100

# 查看前端日志
docker-compose logs -f frontend --tail=100

# 查看 Worker 日志
docker-compose logs -f worker --tail=100

# 导出日志
docker-compose logs backend > backend.log
```

### 监控告警

```yaml
# Prometheus 配置示例
scrape_configs:
  - job_name: 'claw-export-backend'
    static_configs:
      - targets: ['backend:8080']
    metrics_path: '/metrics'
```

### 扩容缩容

```bash
# 增加 Worker 并发数（修改 .env 中的 WORKER_COUNT）
docker-compose up -d worker

# 水平扩展前端
docker-compose up -d --scale frontend=3
```

### 常见问题

#### 1. MySQL 连接失败

```bash
# 检查 MySQL 状态
docker-compose logs mysql

# 重启 MySQL
docker-compose restart mysql

# 检查网络
docker network inspect claw-network
```

#### 2. Redis 连接失败

```bash
# 测试 Redis 连接
docker exec -it claw-redis redis-cli -a ${REDIS_PASSWORD} ping

# 检查 Redis 日志
docker-compose logs redis
```

#### 3. 前端无法访问后端 API

```bash
# 检查 Nginx 配置
docker exec -it claw-frontend nginx -t

# 检查后端健康状态
curl http://localhost:8080/health
```

#### 4. AES 加密密钥长度错误

**错误信息**：`AES_KEY must be 16, 24, or 32 bytes`

**解决方案**：确保 `AES_KEY` 环境变量正好是 32 字节：

```bash
# 检查密钥长度
echo -n "your-aes-key" | wc -c

# 生成正确的 32 字节密钥
openssl rand -base64 32 | head -c 32
```

#### 5. 管理员登录失败（密码错误）

**错误信息**：`{"code":40101,"message":"用户名或密码错误"}`

**解决方案**：重新生成密码哈希并更新数据库：

```bash
# 使用 htpasswd 生成 bcrypt 哈希
htpasswd -nbBC 10 admin admin123
# 输出示例：admin:$2y$10$xxxxx...

# 复制 $2y$10$ 后面的部分，更新数据库
docker exec claw-mysql mysql -uroot -proot123 claw_export -e \
  "UPDATE admins SET password_hash='\$2y\$10\$xxxxx...' WHERE username='admin';"
```

#### 6. 数据库表不存在 / 字段缺失

**错误信息**：`Table 'claw_export.admins' doesn't exist` 或 `Unknown column 'code' in 'field list'`

**解决方案**：执行数据库迁移：

```bash
# 方式一：使用部署脚本
./deploy.sh migrate

# 方式二：手动执行完整初始化 SQL（适合新库）
docker exec -i claw-mysql mysql -uroot -proot123 claw_export < backend/migrations/up/000001_init_schema.up.sql
```

#### 7. Go 版本不兼容

**错误信息**：`go.mod requires go >= 1.25.0`

**解决方案**：

```bash
# 检查本地 Go 版本
go version

# 更新 Dockerfile 中的 Go 版本
# FROM golang:1.25-alpine AS builder
```

#### 8. 任务一直处于"待处理"状态

**原因**：Worker 服务未启动或异常

**解决方案**：

```bash
# 检查 Worker 状态
docker-compose ps worker

# 查看 Worker 日志
docker-compose logs worker --tail=100

# 重启 Worker
docker-compose restart worker
```

#### 9. Dumpling 执行失败：no such file or directory

**错误信息**：`fork/exec /usr/local/bin/dumpling: no such file or directory`

**原因**：
1. `HOST_DUMPLING_PATH` 环境变量未正确配置
2. 使用 sudo 时环境变量未传递
3. Dumpling 需要 glibc，但 Alpine 容器默认使用 musl

**解决方案**：

```bash
# 1. 确认 Dumpling 路径
which dumpling
# 或
tiup list --installed | grep dumpling

# 2. 更新 .env 文件
echo "HOST_DUMPLING_PATH=/home/work/.tiup/components/dumpling/v8.5.5/dumpling" >> .env

# 3. 确保文件可执行
chmod +x $HOST_DUMPLING_PATH

# 4. 重新部署（deploy.sh 已处理 sudo 环境变量传递）
./deploy.sh start

# 5. 验证容器内 dumpling 是否可用
docker exec claw-worker /usr/local/bin/dumpling --version
```

#### 10. Dumpling 执行失败：glibc 兼容问题

**错误信息**：`sh: /usr/local/bin/dumpling: not found`（但文件存在）

**原因**：Alpine Linux 使用 musl libc，而 Dumpling 依赖 glibc

**解决方案**：Dockerfile.worker 已安装 `gcompat`，重新构建镜像：

```bash
docker-compose build --no-cache worker
docker-compose up -d worker
```

---

## 更新升级

### 滚动更新

```bash
# 1. 拉取最新代码
git pull origin main

# 2. 备份数据
./deploy.sh backup

# 3. 重新构建
docker-compose build

# 4. 滚动更新（零停机）
docker-compose up -d --no-deps --build backend

# 5. 验证更新
curl http://localhost:8080/health
```

### 数据库迁移

数据库迁移在容器首次启动时自动执行，脚本位于 `backend/migrations/init.sh`。

#### 迁移文件结构

```
backend/migrations/
├── init.sh                          # Docker 首次启动时按顺序执行 up/*.sql
├── up/
│   └── 000001_init_schema.up.sql    # 最新完整基线：表结构 + 默认管理员
└── down/
    └── 000001_init_schema.down.sql  # 回滚脚本
```

#### 手动执行迁移

```bash
# 本地 Docker
./deploy.sh migrate

# 生产环境（推荐）
cd backend
go run . -action up -dir migrations/up

# 查看迁移状态
cd backend
go run . -action status -dir migrations/up
```

#### 添加新迁移

1. 创建新的迁移文件：

```bash
# backend/migrations/up/000004_add_new_table.up.sql
CREATE TABLE IF NOT EXISTS new_table (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    -- ...
);
```

2. 无需手工修改 `init.sh`，Docker 初始化脚本会自动顺序执行 `up/*.sql`

3. 本地执行 `./deploy.sh migrate` 或生产执行 `go run . -action up -dir migrations/up`

---

## 安全加固

### 1. 修改默认密码

```bash
# 修改 MySQL 密码
docker exec -it claw-mysql mysql -u root -p
ALTER USER 'claw'@'%' IDENTIFIED BY 'new-password';

# 修改 Redis 密码
# 在 .env 中修改 REDIS_PASSWORD，然后重启
docker-compose restart redis
```

### 2. 配置 HTTPS

```bash
# 使用 Let's Encrypt
certbot --nginx -d your-domain.com

# 或在 Nginx 中配置 SSL
```

### 3. 防火墙配置

```bash
# 只开放必要端口
ufw allow 80/tcp
ufw allow 443/tcp
ufw enable
```

---

## Docker Compose 兼容性说明

项目兼容 Docker Compose 1.18.0+，主要注意事项：

| 特性 | 版本要求 | 说明 |
|------|----------|------|
| `version: '3'` | 1.10+ | 使用基础版本确保兼容性 |
| `healthcheck` | 1.10+ | 支持健康检查 |
| `depends_on` | 所有版本 | 仅表示启动顺序，不等待健康状态 |
| `volumes` | 所有版本 | 命名卷和绑定挂载均支持 |

> **注意**：
> - 由于旧版本 `depends_on` 不支持 `condition: service_healthy`，服务启动顺序由 `deploy.sh` 脚本中的等待逻辑保证。
> - 使用 `sudo` 执行 docker-compose 时，环境变量需要显式传递。`deploy.sh` 已处理此问题。

---

## 联系支持

- GitHub Issues: <repository-url>/issues
- 文档: docs/
- 邮件: support@example.com

---

## 多云存储配置

### 支持的存储厂商

| 厂商 | Provider 值 | SDK |
|------|-------------|-----|
| AWS S3 | `aws` | aws-sdk-go-v2 |
| 阿里云 OSS | `aliyun` | aliyun-oss-go-sdk |

### 配置方式

1. **前端配置**：在「S3 配置管理」页面创建配置时选择厂商
2. **API 配置**：通过 `/admin/s3-configs` 接口创建，传递 `provider` 字段

### 阿里云 OSS 配置示例

```bash
# Endpoint 格式
oss-cn-hangzhou.aliyuncs.com

# Region（可选）
cn-hangzhou

# Bucket
your-bucket-name
```

### AWS S3 配置示例

```bash
# Endpoint 格式
https://s3.amazonaws.com

# Region
us-east-1

# Bucket
your-bucket-name
```

---

## 功能更新部署指南

当有新功能更新时，按以下步骤部署：

### 1. 拉取最新代码

```bash
cd /path/to/claw-export-platform
git pull origin main
```

### 2. 执行数据库迁移

```bash
# 使用部署脚本
./deploy.sh migrate

# 或手动执行完整初始化 SQL
docker exec -i claw-mysql mysql -uroot -p${MYSQL_ROOT_PASSWORD} claw_export < backend/migrations/up/000001_init_schema.up.sql
```

### 3. 重新构建并部署

```bash
# 方式一：使用部署脚本（推荐）
./deploy.sh restart

# 方式二：手动操作
docker-compose build --no-cache backend worker
docker-compose up -d backend worker
```

### 4. 验证更新

```bash
# 检查服务状态
docker-compose ps

# 检查后端日志
docker-compose logs -f backend --tail=50

# 检查 Worker 日志
docker-compose logs -f worker --tail=50

# 健康检查
curl http://localhost:8080/health
```

### 5. 验证新功能

登录前端管理界面，在「S3 配置管理」中验证：
- 创建配置时可以选择「AWS S3」或「阿里云 OSS」
- 已有配置显示厂商标签

