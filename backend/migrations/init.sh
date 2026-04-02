#!/bin/bash
# 数据库初始化脚本
# 此脚本会在 MySQL 容器首次启动时自动执行，并顺序执行 up 目录中的 SQL 文件

set -euo pipefail

MIGRATION_DIR="/docker-entrypoint-initdb.d/up"

echo "=========================================="
echo "Claw Export Platform - Database Init"
echo "=========================================="

until mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &> /dev/null; do
    echo "Waiting for MySQL to be ready..."
    sleep 2
done

if [ ! -d "${MIGRATION_DIR}" ]; then
    echo "Migration directory not found: ${MIGRATION_DIR}"
    exit 1
fi

echo "MySQL is ready, applying SQL migrations from ${MIGRATION_DIR} ..."

find "${MIGRATION_DIR}" -maxdepth 1 -type f -name '*.up.sql' | sort | while read -r migration_file; do
    filename=$(basename "${migration_file}")
    echo "Applying migration: ${filename}"
    mysql -u root -p"${MYSQL_ROOT_PASSWORD}" "${MYSQL_DATABASE}" < "${migration_file}"
done

echo "=========================================="
echo "Database initialization completed!"
echo "=========================================="
