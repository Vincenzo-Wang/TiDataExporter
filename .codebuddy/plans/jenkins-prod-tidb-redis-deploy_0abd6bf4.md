---
name: jenkins-prod-tidb-redis-deploy
overview: 为当前 MVP 项目设计并实施一套新的生产上线方案：移除本地 MySQL/Redis 依赖，改为阿里云 TiDB 与云 Redis，补齐 TiDB 初始化 SQL 与幂等升级脚本，并新增基于 Jenkins + SSH + systemd + Nginx 的发布链路。
todos:
  - id: freeze-scope
    content: 使用 [skill:brainstorming] 与 [subagent:code-explorer] 固化生产边界和改动清单
    status: completed
  - id: unify-migrations
    content: 使用 [skill:code-simplifier] 收敛 TiDB 基线、升级 SQL 和默认管理员初始化
    status: completed
    dependencies:
      - freeze-scope
  - id: wire-runtime-config
    content: 改造 config.go、database.go、redis.go 与 server/worker 启动参数
    status: completed
    dependencies:
      - unify-migrations
  - id: build-prod-assets
    content: 新增 systemd、Nginx、env 模板和版本化发布脚本
    status: completed
    dependencies:
      - wire-runtime-config
  - id: jenkins-pipeline
    content: 新增 Jenkinsfile，打通构建、发布、迁移、健康检查和回滚
    status: completed
    dependencies:
      - unify-migrations
      - build-prod-assets
  - id: update-docs
    content: 更新 README 与 DEVELOPMENT 生产上线文档
    status: completed
    dependencies:
      - jenkins-pipeline
---

## User Requirements

- 现有版本已可运行，当前本地部署包含前台、接口、任务执行、数据库、缓存共 5 个服务。
- 需要新增一套生产上线方案，目标为单台 Linux 服务器；前台以静态站点方式提供访问，两个后端进程以系统服务方式常驻运行。
- 生产环境不再自建本地数据库和缓存，改为接入阿里云托管数据库与缓存服务。
- 需要整理一份完整数据库初始化脚本，包含完整表结构、默认管理员初始化、可重复执行的升级脚本，并兼容目标数据库的建表选项、自增行为和表注释要求。
- 需要配套自动化上线流程，覆盖构建、发布、启动、健康检查与失败回滚。

## Product Overview

提供一套独立于本地容器启动方式的生产发布方案。上线后页面通过统一站点访问，接口与任务执行进程在后台稳定运行，数据存储和队列依赖云服务，初始化与升级脚本可独立执行。

## Core Features

- 单机生产发布与版本切换
- 云数据库与云缓存接入
- 完整初始化脚本与幂等升级
- 服务托管、健康检查与回滚

## Tech Stack Selection

- 后端沿用现有 Go、Gin、GORM、MySQL 协议驱动实现；应用元数据库从本地 MySQL 容器切到阿里云 TiDB。
- 队列和缓存沿用现有 go-redis 实现；生产环境切到阿里云 Redis。
- 前端沿用现有 React 和 Vite 构建产物，生产由主机 Nginx 提供静态资源和反向代理。
- 生产托管方式按已确认方案落到单机 Linux：Nginx 提供站点，backend 和 worker 通过 systemd 常驻。
- 自动化上线采用 Jenkins Pipeline；复用当前仓库已有的测试和构建逻辑，不把生产流程塞回现有 docker-compose 本地链路。

## Implementation Approach

采用“开发和生产双轨”方案：保留 `/Users/admin/CodeBuddy/Claw/docker-compose.yml` 和 `/Users/admin/CodeBuddy/Claw/deploy.sh` 作为本地 MVP 启动方式，新增独立的生产发布资产目录与 Jenkins 流水线。Jenkins 负责构建前端静态产物和 Linux 二进制、打包发布物、通过 SSH 投递到目标机、执行数据库迁移、切换版本目录并重启服务。

数据库初始化以现有 `backend/migrations/up/*.sql` 为唯一可信来源，改造成 TiDB 兼容的完整基线加幂等增量脚本，不再以 `backend/migrations/init.sh` 的内嵌 SQL 和 `backend/main.go` 的 GORM AutoMigrate 作为生产主路径。这样能避免当前三套初始化机制并存带来的漂移风险。

关键技术决策：

- 生产不再使用本地 mysql 和 redis 容器，避免与云服务配置重复维护。
- `backend/config/config.go` 继续作为统一配置入口，补齐当前未真正生效的 `SERVER_PORT`、`WORKER_COUNT`、工作目录、TLS 和连接池参数，避免 systemd 依赖额外壳脚本拼 flag。
- `backend/services/export/executor.go` 需要消费已存在但未落地的 `ssl_mode` 字段，避免云上 TiDB 导出链路因 TLS 缺失失败。
- TiDB 初始化表结构统一补全 `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci`、表注释和 `/*T![auto_id_cache] AUTO_ID_CACHE=1 */`。`AUTO_INCREMENT=8184` 只作为格式参考，不建议对全新库硬编码固定高位值；新库默认从 1 起步，只有迁移存量数据时才保留历史高水位。

性能和可靠性：

- SQL 初始化和升级执行复杂度为 O(迁移文件数)，单次成本主要取决于表数量和索引创建。
- Jenkins 主要瓶颈在前端依赖安装、Go 编译和制品上传；通过 Jenkins 缓存 Go module 和 npm 缓存、压缩 release 包、只上传版本制品降低耗时。
- 运行期继续复用 `backend/pkg/database/database.go` 的连接池和慢 SQL 日志约定，新增云库连接参数后仍保持低侵入。
- 发布采用版本化目录加 symlink 切换，应用切换失败可快速回退到上一个 release；数据库迁移按前向兼容设计，避免回滚时依赖危险的 DDL 回退。

## Implementation Notes

- 当前 `.env` 和脚本存在 `ENCRYPTION_KEY` 与 `AES_KEY` 不一致问题；示例配置和部署文档必须统一为代码真实使用的 `AES_KEY`。
- 当前 `backend/cmd/server/main.go` 仅使用 flag 里的端口，`backend/cmd/worker/main.go` 仅使用 flag 里的 worker 数；这会导致 systemd 环境变量配置失效，必须先修正。
- 当前 `backend/pkg/redis/redis.go` 未支持用户名和 TLS，接入阿里云 Redis 时需要补齐可选用户名、TLS 开关和证书校验策略。
- 当前 `deploy.sh` 的迁移策略依赖“报错可忽略”；生产迁移要改成真正幂等并失败即中止，避免脏发布。
- 保持本地开发链路可用，不做与生产无关的重构；生产资产统一放到新目录，控制影响面。

## Architecture Design

### Runtime Structure

- Nginx 直接服务前端 `dist`，并把 `/api` 代理到本机 backend。
- backend 和 worker 都从 systemd 读取环境变量文件，连接阿里云 TiDB 与阿里云 Redis。
- Jenkins 负责生成 release 包并远程发布到目标机的版本目录。
- 迁移在应用切换前执行，成功后再切换 `current` 并重启服务。

### Deployment Flow

1. Jenkins 拉取代码并执行后端测试、前端构建。
2. Jenkins 构建 backend、worker、migrate 三个 Linux 二进制，连同前端 dist、迁移 SQL、systemd 和 Nginx 模板一起打包。
3. Jenkins 通过 SSH 上传到目标机的 release 目录。
4. 远程执行迁移二进制，按顺序跑 `backend/migrations/up/*.sql`。
5. 切换 `current` 软链接，reload Nginx，restart backend 和 worker。
6. 检查 `backend /health`、站点可达性和 systemd 状态；失败则回滚到上一个 release。

## Directory Structure

### Summary

本次改造新增生产部署资产和 Jenkins 流水线，同时收敛现有配置、迁移和启动入口的重复逻辑；本地 Docker 启动方式继续保留。

- `/Users/admin/CodeBuddy/Claw/Jenkinsfile`  [NEW] Jenkins 生产流水线。实现检出、测试、前端构建、后端多二进制编译、制品打包、SSH 发布、远程迁移、服务重启、健康检查和失败回滚。必须使用 Jenkins Credentials 管理主机、密钥和环境变量。
- `/Users/admin/CodeBuddy/Claw/deploy/systemd/claw-backend.service`  [NEW] backend 的 systemd 单元文件。负责加载 `backend.env`、指向版本化工作目录、定义重启策略、优雅停止和日志输出约束。
- `/Users/admin/CodeBuddy/Claw/deploy/systemd/claw-worker.service`  [NEW] worker 的 systemd 单元文件。负责加载 `worker.env`、传入 worker 数和工作目录，确保 SIGTERM 下优雅退出。
- `/Users/admin/CodeBuddy/Claw/deploy/nginx/claw.conf`  [NEW] 主机 Nginx 站点配置。复用现有 `frontend/nginx.conf` 的 SPA 回退、gzip、`/api` 代理和健康检查语义，并补充静态资源缓存分层策略。
- `/Users/admin/CodeBuddy/Claw/deploy/env/backend.env.example`  [NEW] backend 生产环境变量模板。包含云 TiDB、云 Redis、TLS、连接池、`AES_KEY`、`JWT_SECRET`、监听端口等配置。
- `/Users/admin/CodeBuddy/Claw/deploy/env/worker.env.example`  [NEW] worker 生产环境变量模板。包含云 TiDB、云 Redis、`DUMPLING_PATH`、`WORKER_COUNT`、工作目录等配置。
- `/Users/admin/CodeBuddy/Claw/deploy/scripts/package_release.sh`  [NEW] release 打包脚本。输出版本化 tar.gz，内含前端 dist、backend 和 worker 二进制、迁移执行二进制、SQL 文件和部署模板。
- `/Users/admin/CodeBuddy/Claw/deploy/scripts/deploy_remote.sh`  [NEW] 远程发布脚本。负责解压到 releases 目录、执行迁移、切换 symlink、reload 服务并做健康检查。
- `/Users/admin/CodeBuddy/Claw/deploy/scripts/rollback_remote.sh`  [NEW] 远程回滚脚本。负责回退到上一版本目录并重启服务，不触碰数据库 destructive rollback。
- `/Users/admin/CodeBuddy/Claw/backend/migrations/up/000001_init_schema.up.sql`  [MODIFY] 主基线建表脚本。补齐 TiDB 表选项、字符集、排序规则、表注释和最新字段定义，使新库初始化不再依赖后续补丁才能完整可用。
- `/Users/admin/CodeBuddy/Claw/backend/migrations/up/000003_add_missing_columns.up.sql`  [MODIFY] 旧库升级脚本。改成真正幂等的列补齐逻辑，避免重复执行时报错。
- `/Users/admin/CodeBuddy/Claw/backend/migrations/up/000004_add_s3_provider.up.sql`  [MODIFY] 旧库升级脚本。改成幂等的字段补丁和数据回填逻辑，兼容重复执行。
- `/Users/admin/CodeBuddy/Claw/backend/migrations/up/000005_seed_default_admin.up.sql`  [NEW] 默认管理员和必要基础数据脚本。复用当前 `init.sh` 中已验证的管理员 bcrypt 哈希，确保 fresh install 直接可登录且可重复执行。
- `/Users/admin/CodeBuddy/Claw/backend/migrations/init.sh`  [MODIFY] MySQL 容器初始化脚本。改为顺序执行 `up` 目录中的 SQL 文件，去掉内嵌 DDL 重复内容，保持本地 Docker 与生产迁移来源一致。
- `/Users/admin/CodeBuddy/Claw/backend/main.go`  [MODIFY] 迁移 CLI。由 AutoMigrate 迁移为 SQL 文件执行器，用于 Jenkins 远程发布阶段连接云 TiDB 执行升级。
- `/Users/admin/CodeBuddy/Claw/backend/config/config.go`  [MODIFY] 配置中心。补齐 DB 和 Redis 的 TLS、用户名、连接池与 worker 配置，统一 `AES_KEY` 和端口等环境变量定义。
- `/Users/admin/CodeBuddy/Claw/backend/pkg/database/database.go`  [MODIFY] 云 TiDB 连接实现。支持可配置 TLS DSN、超时与连接池参数，保留现有 GORM 与慢 SQL 日志模式。
- `/Users/admin/CodeBuddy/Claw/backend/pkg/redis/redis.go`  [MODIFY] 云 Redis 连接实现。补充用户名、TLS、池参数和更稳健的连通性校验。
- `/Users/admin/CodeBuddy/Claw/backend/cmd/server/main.go`  [MODIFY] API 启动入口。改为优先消费统一配置，确保 systemd 环境变量配置直接生效。
- `/Users/admin/CodeBuddy/Claw/backend/cmd/worker/main.go`  [MODIFY] worker 启动入口。让 `WORKER_COUNT` 和工作目录在生产环境真实生效，并保持现有优雅退出流程。
- `/Users/admin/CodeBuddy/Claw/backend/services/export/executor.go`  [MODIFY] Dumpling 执行链路。把已有 `ssl_mode` 字段映射到 Dumpling 参数，降低云上导出失败风险。
- `/Users/admin/CodeBuddy/Claw/.env.example`  [MODIFY] 本地环境示例。修正 `AES_KEY` 命名、澄清本地容器用途，避免与生产模板混用。
- `/Users/admin/CodeBuddy/Claw/deploy.sh`  [MODIFY] 本地部署脚本。只保留开发和联调用途，修正对 `AES_KEY` 的校验和迁移入口说明，不引入生产逻辑。
- `/Users/admin/CodeBuddy/Claw/README.md`  [MODIFY] 顶层说明。明确区分“本地 Docker”与“生产 Jenkins 加 systemd 加 Nginx”两条部署路线。
- `/Users/admin/CodeBuddy/Claw/docs/DEVELOPMENT.md`  [MODIFY] 详细部署手册。补充服务器目录规范、systemd 安装、Nginx 启用、Jenkins 参数、上线与回滚步骤。

## Agent Extensions

### Skill

- **brainstorming**
- Purpose: 在改造前收敛生产部署边界、迁移策略和回滚规则
- Expected outcome: 形成不干扰本地容器流程的生产上线方案
- **code-simplifier**
- Purpose: 收敛现有三套初始化和配置入口中的重复逻辑
- Expected outcome: 降低 SQL、脚本和环境变量的重复维护成本

### SubAgent

- **code-explorer**
- Purpose: 二次核对配置读取、迁移执行链路和启动入口
- Expected outcome: 受影响文件和依赖调用点覆盖完整，不遗漏发布风险