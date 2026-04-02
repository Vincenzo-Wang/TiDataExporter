#!/usr/bin/env bash

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.test}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.test.yml}"
BACKEND_HEALTH_PATH="${BACKEND_HEALTH_PATH:-/health}"
FRONTEND_HEALTH_PATH="${FRONTEND_HEALTH_PATH:-/health}"
FRONTEND_API_HEALTH_PATH="${FRONTEND_API_HEALTH_PATH:-/api/health}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

USE_SUDO=0

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok() { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_err() { echo -e "${RED}[ERR]${NC} $*"; }

usage() {
  cat <<EOF
Claw 测试环境一键发布脚本

用法:
  ./release.sh <mode>

mode:
  check           校验 compose 配置并查看服务状态
  frontend        仅发布前端
  backend         仅发布后端
  backend-worker  发布后端+worker
  full            发布 backend+worker+frontend
  migrate-only    仅执行 migrate up
  with-migrate    先 migrate up，再发布 backend+worker+frontend
  logs            查看 backend/worker/frontend 最近日志

可选环境变量:
  ENV_FILE=.env.test
  COMPOSE_FILE=docker-compose.test.yml
EOF
}

extract_env_value() {
  local key="$1"
  local default_val="$2"

  if [[ ! -f "$ENV_FILE" ]]; then
    echo "$default_val"
    return
  fi

  local line
  line=$(grep -E "^${key}=" "$ENV_FILE" | tail -n 1 || true)
  if [[ -z "$line" ]]; then
    echo "$default_val"
  else
    echo "${line#*=}"
  fi
}

require_tools() {
  command -v docker >/dev/null 2>&1 || {
    log_err "未找到 docker"
    exit 1
  }

  if docker info >/dev/null 2>&1; then
    USE_SUDO=0
  elif sudo -n docker info >/dev/null 2>&1 || sudo docker info >/dev/null 2>&1; then
    USE_SUDO=1
    log_warn "当前将使用 sudo 执行 docker"
  else
    log_err "docker 不可用，请检查 docker 服务和权限"
    exit 1
  fi
}

docker_cmd() {
  if [[ "$USE_SUDO" -eq 1 ]]; then
    sudo docker "$@"
  else
    docker "$@"
  fi
}

compose_cmd() {
  docker_cmd compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

precheck() {
  [[ -f "$ENV_FILE" ]] || {
    log_err "未找到环境文件: $ENV_FILE"
    exit 1
  }

  [[ -f "$COMPOSE_FILE" ]] || {
    log_err "未找到 compose 文件: $COMPOSE_FILE"
    exit 1
  }

  compose_cmd config >/tmp/claw-compose-check.out
  log_ok "compose 配置校验通过"
}

run_migrate() {
  log_info "执行数据库迁移..."
  compose_cmd run --rm migrate -action up -dir /app/migrations/up
  log_ok "迁移完成"
}

deploy_services() {
  local services=("$@")
  log_info "发布服务: ${services[*]}"
  compose_cmd up -d --build "${services[@]}"
  log_ok "发布完成: ${services[*]}"
}

show_ps() {
  compose_cmd ps
}

health_check() {
  local backend_port frontend_port
  backend_port=$(extract_env_value "BACKEND_HOST_PORT" "18081")
  frontend_port=$(extract_env_value "FRONTEND_PORT" "18080")

  log_info "健康检查 backend: http://127.0.0.1:${backend_port}${BACKEND_HEALTH_PATH}"
  if curl -fsS "http://127.0.0.1:${backend_port}${BACKEND_HEALTH_PATH}" >/dev/null; then
    log_ok "backend 健康"
  else
    log_warn "backend 健康检查失败，请查看日志"
  fi

  log_info "健康检查 frontend: http://127.0.0.1:${frontend_port}${FRONTEND_HEALTH_PATH}"
  if curl -fsS "http://127.0.0.1:${frontend_port}${FRONTEND_HEALTH_PATH}" >/dev/null; then
    log_ok "frontend 健康"
  else
    log_warn "frontend 健康检查失败，请查看日志"
  fi

  log_info "健康检查 frontend -> backend 反代链路: http://127.0.0.1:${frontend_port}${FRONTEND_API_HEALTH_PATH}"
  if curl -fsS "http://127.0.0.1:${frontend_port}${FRONTEND_API_HEALTH_PATH}" >/dev/null; then
    log_ok "frontend -> backend 反代链路健康"
  else
    log_warn "frontend -> backend 反代链路检查失败，可能存在 upstream 解析或容器网络问题"
  fi
}

show_logs() {
  compose_cmd logs --no-color --tail=200 backend worker frontend
}

main() {
  local mode="${1:-}"

  if [[ -z "$mode" ]] || [[ "$mode" == "-h" ]] || [[ "$mode" == "--help" ]] || [[ "$mode" == "help" ]]; then
    usage
    exit 0
  fi

  require_tools
  precheck

  case "$mode" in
    check)
      show_ps
      health_check
      ;;
    frontend)
      deploy_services frontend
      show_ps
      health_check
      ;;
    backend)
      deploy_services backend
      show_ps
      health_check
      ;;
    backend-worker)
      deploy_services backend worker
      show_ps
      health_check
      ;;
    full)
      deploy_services backend worker frontend
      show_ps
      health_check
      ;;
    migrate-only)
      run_migrate
      ;;
    with-migrate)
      run_migrate
      deploy_services backend worker frontend
      show_ps
      health_check
      ;;
    logs)
      show_logs
      ;;
    *)
      log_err "未知模式: $mode"
      usage
      exit 1
      ;;
  esac
}

main "$@"
