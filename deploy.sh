#!/bin/bash

# ============================================
# Claw Export Platform - 一键部署脚本
# ============================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Docker 运行方式
USE_SUDO_DOCKER=0
COMPOSE_COMMAND=""

# 检查命令是否存在
check_command() {
    if ! command -v "$1" &> /dev/null; then
        log_error "$1 未安装，请先安装 $1"
        exit 1
    fi
}

# 执行 docker 命令（自动处理 sudo）
docker_cmd() {
    if [ "$USE_SUDO_DOCKER" -eq 1 ]; then
        sudo docker "$@"
    else
        docker "$@"
    fi
}

# 执行 compose 命令（兼容 docker-compose 和 docker compose）
compose_cmd() {
    if [ "$COMPOSE_COMMAND" = "docker-compose" ]; then
        if [ "$USE_SUDO_DOCKER" -eq 1 ]; then
            sudo docker-compose "$@"
        else
            docker-compose "$@"
        fi
    else
        docker_cmd compose "$@"
    fi
}

# 检测 Docker 与 Compose 运行方式
detect_docker_mode() {
    check_command docker

    # 检测 compose 命令类型
    if command -v docker-compose &> /dev/null; then
        COMPOSE_COMMAND="docker-compose"
    elif docker compose version &> /dev/null || sudo -n docker compose version &> /dev/null || sudo docker compose version &> /dev/null; then
        COMPOSE_COMMAND="docker compose"
    else
        log_error "未检测到 docker-compose 或 docker compose，请先安装 Docker Compose"
        exit 1
    fi

    # 检测 docker 是否需要 sudo
    if docker info &> /dev/null; then
        USE_SUDO_DOCKER=0
    elif sudo -n docker info &> /dev/null || sudo docker info &> /dev/null; then
        USE_SUDO_DOCKER=1
        log_warning "检测到当前用户需要 sudo 执行 docker，后续将自动使用 sudo"
    else
        log_error "无法执行 docker 命令，请检查 Docker 是否运行或当前用户 sudo 权限"
        exit 1
    fi

    # 校验 compose 是否可用
    if [ "$COMPOSE_COMMAND" = "docker-compose" ]; then
        if [ "$USE_SUDO_DOCKER" -eq 1 ]; then
            sudo docker-compose version &> /dev/null || {
                log_error "检测到 docker-compose，但 sudo 无法执行 docker-compose"
                exit 1
            }
        else
            docker-compose version &> /dev/null || {
                log_error "docker-compose 不可用"
                exit 1
            }
        fi
    else
        compose_cmd version &> /dev/null || {
            log_error "docker compose 不可用"
            exit 1
        }
    fi
}

# 检查环境
check_environment() {
    log_info "检查环境..."

    detect_docker_mode

    # 检查 Docker 是否运行
    if ! docker_cmd info &> /dev/null; then
        log_error "Docker 未运行，请先启动 Docker"
        exit 1
    fi

    log_success "环境检查通过"
}

# 检查配置文件
check_config() {
    log_info "检查配置文件..."
    
    if [ ! -f .env ]; then
        if [ -f .env.example ]; then
            log_warning ".env 文件不存在，从 .env.example 复制..."
            cp .env.example .env
            log_warning "请修改 .env 文件中的配置，特别是密码和密钥！"
            read -p "是否现在编辑配置文件？(y/n): " edit_config
            if [ "$edit_config" = "y" ]; then
                ${EDITOR:-vim} .env
            fi
        else
            log_error ".env 和 .env.example 文件都不存在"
            exit 1
        fi
    fi
    
    # 检查必要的环境变量
    source .env
    
    if [ -z "$JWT_SECRET" ] || [ "$JWT_SECRET" = "your-super-secret-jwt-key-change-in-production" ]; then
        log_warning "JWT_SECRET 未配置或使用默认值，建议修改！"
    fi
    
    if [ -z "$ENCRYPTION_KEY" ] || [ "$ENCRYPTION_KEY" = "your-32-byte-encryption-key-here" ]; then
        log_warning "ENCRYPTION_KEY 未配置或使用默认值，建议修改！"
    fi
    
    log_success "配置文件检查完成"
}

# 创建必要的目录
create_directories() {
    log_info "创建必要的目录..."
    
    mkdir -p backend/logs
    mkdir -p backend/tmp
    mkdir -p mysql_data
    mkdir -p redis_data
    
    log_success "目录创建完成"
}

# 构建镜像
build_images() {
    log_info "构建 Docker 镜像..."
    
    compose_cmd build --no-cache
    
    log_success "镜像构建完成"
}

# 启动服务
start_services() {
    log_info "启动服务..."
    
    # 停止旧容器
    compose_cmd down --remove-orphans 2>/dev/null || true
    
    # 创建网络（如果不存在）
    log_info "检查 Docker 网络..."
    if ! docker_cmd network inspect claw-network &> /dev/null; then
        log_info "创建 Docker 网络 claw-network..."
        docker_cmd network create claw-network
    else
        log_info "Docker 网络 claw-network 已存在"
    fi
    
    # 启动 MySQL
    log_info "启动 MySQL..."
    compose_cmd up -d mysql
    
    # 等待 MySQL 就绪
    log_info "等待 MySQL 就绪..."
    sleep 30
    until compose_cmd exec -T mysql mysqladmin ping -h localhost --silent; do
        log_info "等待 MySQL..."
        sleep 5
    done
    log_success "MySQL 已就绪"
    
    # 启动 Redis
    log_info "启动 Redis..."
    compose_cmd up -d redis
    
    # 等待 Redis 就绪
    log_info "等待 Redis 就绪..."
    sleep 10
    until compose_cmd exec -T redis redis-cli -a ${REDIS_PASSWORD:-redis123} ping | grep -q PONG; do
        log_info "等待 Redis..."
        sleep 2
    done
    log_success "Redis 已就绪"
    
    # 启动后端
    log_info "启动后端服务..."
    compose_cmd up -d backend
    
    # 等待后端就绪
    log_info "等待后端服务就绪..."
    sleep 10
    until curl -s http://localhost:8080/health > /dev/null; do
        log_info "等待后端服务..."
        sleep 3
    done
    log_success "后端服务已就绪"
    
    # 启动前端
    log_info "启动前端服务..."
    compose_cmd up -d frontend
    
    # 等待前端就绪
    sleep 5
    log_success "前端服务已就绪"
    
    # 启动 Worker
    log_info "启动 Worker 服务..."
    compose_cmd up -d worker
    
    # 等待 Worker 就绪
    sleep 5
    log_success "Worker 服务已就绪"
    
    log_success "所有服务启动完成！"
}

# 显示状态
show_status() {
    log_info "服务状态："
    echo ""
    compose_cmd ps
    echo ""

    log_success "=========================================="
    log_success "部署完成！"
    log_success "=========================================="
    echo ""
    echo -e "${GREEN}前端地址：${NC}http://localhost:${FRONTEND_PORT:-80}"
    echo -e "${GREEN}后端地址：${NC}http://localhost:${BACKEND_PORT:-8080}"
    echo -e "${GREEN}默认账号：${NC}admin / admin123"
    echo ""
    log_warning "请尽快修改默认密码和密钥！"
    echo ""
}

# 健康检查
health_check() {
    log_info "执行健康检查..."
    
    # 检查后端
    if curl -s http://localhost:8080/health > /dev/null; then
        log_success "后端服务健康"
    else
        log_error "后端服务不健康"
    fi
    
    # 检查前端
    if curl -s http://localhost:80/health > /dev/null; then
        log_success "前端服务健康"
    else
        log_error "前端服务不健康"
    fi
}

# 显示帮助
show_help() {
    echo "Claw Export Platform 部署脚本"
    echo ""
    echo "用法: ./deploy.sh [命令]"
    echo ""
    echo "命令:"
    echo "  start       启动所有服务（默认）"
    echo "  stop        停止所有服务"
    echo "  restart     重启所有服务"
    echo "  status      查看服务状态"
    echo "  logs        查看日志"
    echo "  build       构建镜像"
    echo "  clean       清理所有容器和卷"
    echo "  backup      备份数据库"
    echo "  help        显示帮助信息"
    echo ""
}

# 停止服务
stop_services() {
    log_info "停止所有服务..."
    compose_cmd down
    log_success "服务已停止"
}

# 清理
clean_all() {
    log_warning "这将删除所有容器、卷和数据！"
    read -p "确认继续？(y/n): " confirm
    if [ "$confirm" = "y" ]; then
        compose_cmd down -v --remove-orphans
        rm -rf mysql_data redis_data
        log_success "清理完成"
    else
        log_info "取消清理"
    fi
}

# 备份数据库
backup_database() {
    log_info "备份数据库..."
    
    BACKUP_FILE="backup_$(date +%Y%m%d_%H%M%S).sql"
    source .env
    
    compose_cmd exec -T mysql mysqldump -u root -p${MYSQL_ROOT_PASSWORD:-root123} \
        --single-transaction \
        --routines \
        --triggers \
        --events \
        ${MYSQL_DATABASE:-claw_export} > "$BACKUP_FILE"
    
    gzip "$BACKUP_FILE"
    
    log_success "备份完成: ${BACKUP_FILE}.gz"
}

# 查看日志
show_logs() {
    if [ -z "$1" ]; then
        compose_cmd logs -f --tail=100
    else
        compose_cmd logs -f --tail=100 "$1"
    fi
}

# 主函数
main() {
    case "${1:-start}" in
        start)
            check_environment
            check_config
            create_directories
            build_images
            start_services
            health_check
            show_status
            ;;
        stop)
            detect_docker_mode
            stop_services
            ;;
        restart)
            check_environment
            stop_services
            start_services
            health_check
            show_status
            ;;
        status)
            detect_docker_mode
            compose_cmd ps
            ;;
        logs)
            detect_docker_mode
            show_logs "$2"
            ;;
        build)
            check_environment
            build_images
            ;;
        clean)
            detect_docker_mode
            clean_all
            ;;
        backup)
            detect_docker_mode
            backup_database
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
