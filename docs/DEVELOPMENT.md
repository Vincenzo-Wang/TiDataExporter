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
- Docker Compose 2.0+

#### 开发环境

- Go 1.25+
- Node.js 18+
- MySQL 8.0+
- Redis 7+

---

## 配置说明

### 环境变量配置 (.env)

```bash
# ============================================
# 应用配置
# ============================================
APP_ENV=production              # 环境：development/staging/production
APP_PORT=8080                   # 后端服务端口

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
JWT_SECRET=your-super-secret-jwt-key-change-in-production-min-32-chars

# AES 加密密钥：必须是正好 32 字节的字符串
# 注意：长度必须正好 32 字节（用于 AES-256 加密）
# 生成方法：openssl rand -base64 32 | head -c 32
AES_KEY=claw-export-aes-key-32-bytes!!!!

# ============================================
# 服务端口
# ============================================
FRONTEND_PORT=80                # 前端服务端口
BACKEND_PORT=8080               # 后端 API 端口

# ============================================
# S3 存储配置（可选，租户可自行配置）
# ============================================
S3_ACCESS_KEY=                  # Access Key ID
S3_SECRET_KEY=                  # Secret Access Key
S3_ENDPOINT=                    # S3 Endpoint（如：https://s3.amazonaws.com）
S3_REGION=us-east-1             # S3 Region
S3_BUCKET=                      # 默认 Bucket 名称

# ============================================
# 日志配置
# ============================================
LOG_LEVEL=info                  # 日志级别：debug/info/warn/error
```

### 后端配置文件 (config/config.yaml)

```yaml
# 应用配置
app:
  env: production              # 环境
  port: 8080                   # 服务端口
  name: claw-export-platform   # 应用名称

# 数据库配置
database:
  host: ${DB_HOST:localhost}   # 支持环境变量覆盖
  port: ${DB_PORT:3306}
  user: ${DB_USER:claw}
  password: ${DB_PASSWORD:claw123}
  name: ${DB_NAME:claw_export}
  max_open_conns: 100          # 最大连接数
  max_idle_conns: 10           # 最大空闲连接
  conn_max_lifetime: 3600      # 连接最大生命周期（秒）

# Redis 配置
redis:
  host: ${REDIS_HOST:localhost}
  port: ${REDIS_PORT:6379}
  password: ${REDIS_PASSWORD:}
  db: 0
  pool_size: 100

# 安全配置
security:
  jwt_secret: ${JWT_SECRET}    # JWT 密钥
  encryption_key: ${ENCRYPTION_KEY}  # 加密密钥
  token_expire_hour: 24        # Token 有效期（小时）
  refresh_token_expire_hour: 168  # 刷新 Token 有效期

# Dumpling 配置
dumpling:
  path: /usr/local/bin/dumpling  # Dumpling 可执行文件路径
  default_threads: 4             # 默认线程数
  default_file_size: "256MiB"    # 默认文件大小
  default_rows: 100000           # 默认行数限制
  timeout: 3600                  # 超时时间（秒）

# Worker 配置
worker:
  pool_size: 10                # Worker 池大小
  queue_size: 1000             # 任务队列大小
  max_retry: 3                 # 最大重试次数
  retry_delay: 60              # 重试延迟（秒）

# 清理任务配置
cleanup:
  enabled: true                # 是否启用清理任务
  schedule: "0 2 * * *"        # Cron 表达式：每天凌晨 2 点执行
  retention_days: 7            # 文件保留天数

# 日志配置
log:
  level: ${LOG_LEVEL:info}
  format: json                 # 日志格式：json/text
  output: stdout               # 输出位置：stdout/file
  file: logs/app.log           # 日志文件路径（output=file 时生效）
  max_size: 100                # 单文件最大大小（MB）
  max_backups: 10              # 最大保留文件数
  max_age: 30                  # 最大保留天数
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

### 方式一：一键部署（推荐）

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

# 3. 执行部署脚本
chmod +x deploy.sh
./deploy.sh

# 4. 查看服务状态
docker-compose ps

# 5. 查看日志（确认初始化成功）
docker-compose logs mysql | grep -A 10 "Database Init"

# 6. 验证登录
curl -X POST http://localhost:8080/api/v1/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

#### 重新初始化数据库

如果需要重新初始化数据库（会删除所有数据）：

```bash
# 停止服务并删除数据卷
docker-compose down -v

# 重新启动（会自动执行初始化）
docker-compose up -d
```

### 方式二：手动部署

```bash
# 1. 创建网络
docker network create claw-network

# 2. 启动 MySQL
docker-compose up -d mysql
# 等待 MySQL 就绪
sleep 30

# 3. 启动 Redis
docker-compose up -d redis
# 等待 Redis 就绪
sleep 10

# 4. 启动后端
docker-compose up -d backend

# 5. 启动前端
docker-compose up -d frontend

# 6. 检查服务
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
```

#### 2. Systemd 服务配置

```ini
# /etc/systemd/system/claw-export.service
[Unit]
Description=Claw Export Platform Backend
After=network.target mysql.service redis.service

[Service]
Type=simple
User=claw
Group=claw
WorkingDirectory=/opt/claw-export
ExecStart=/opt/claw-export/claw-export
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# 环境变量
Environment=APP_ENV=production
Environment=DB_HOST=localhost
Environment=DB_PORT=3306
Environment=DB_USER=claw
Environment=DB_PASSWORD=your-password
Environment=REDIS_HOST=localhost
Environment=REDIS_PORT=6379
Environment=REDIS_PASSWORD=your-redis-password
Environment=JWT_SECRET=your-jwt-secret
Environment=ENCRYPTION_KEY=your-encryption-key

[Install]
WantedBy=multi-user.target
```

```bash
# 启用服务
sudo systemctl daemon-reload
sudo systemctl enable claw-export
sudo systemctl start claw-export

# 查看状态
sudo systemctl status claw-export

# 查看日志
sudo journalctl -u claw-export -f
```

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
        - name: APP_ENV
          value: "production"
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: claw-export-secrets
              key: db-host
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
# 增加 Worker 数量（修改 docker-compose.yml 中的 worker.pool_size）
docker-compose up -d --force-recreate backend

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

#### 6. 数据库表不存在

**错误信息**：`Table 'claw_export.admins' doesn't exist`

**解决方案**：手动执行迁移脚本：

```bash
# 方式一：复制并执行
docker cp backend/migrations/init.sh claw-mysql:/tmp/
docker exec claw-mysql bash /tmp/init.sh

# 方式二：重新初始化（会删除数据）
docker-compose down -v && docker-compose up -d
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

---

## 更新升级

### 滚动更新

```bash
# 1. 拉取最新代码
git pull origin main

# 2. 备份数据
./scripts/backup.sh

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
├── init.sh                    # 自动初始化脚本（Docker 用）
├── up/
│   └── 000001_init_schema.up.sql  # 初始化表结构 SQL
└── down/
    └── 000001_init_schema.down.sql  # 回滚脚本
```

#### 手动执行迁移

```bash
# 方式一：通过 MySQL 容器执行
docker exec claw-mysql bash /docker-entrypoint-initdb.d/01_init.sh

# 方式二：直接执行 SQL
docker exec -i claw-mysql mysql -uroot -proot123 claw_export < backend/migrations/up/000001_init_schema.up.sql
```

#### 添加新迁移

1. 创建新的迁移文件：

```bash
# backend/migrations/up/000002_add_new_table.up.sql
CREATE TABLE IF NOT EXISTS new_table (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    -- ...
);
```

2. 更新 `init.sh` 添加新的 SQL 语句

3. 重新部署服务

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

## 联系支持

- GitHub Issues: <repository-url>/issues
- 文档: docs/
- 邮件: support@example.com
