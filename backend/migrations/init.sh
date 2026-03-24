#!/bin/bash
# 数据库初始化脚本
# 此脚本会在 MySQL 容器首次启动时自动执行
# 内容与 migrations/up/000001_init_schema.up.sql 保持一致

set -e

echo "=========================================="
echo "Claw Export Platform - Database Init"
echo "=========================================="

# 等待 MySQL 完全启动
until mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &> /dev/null; do
    echo "Waiting for MySQL to be ready..."
    sleep 2
done

echo "MySQL is ready, executing migrations..."

# 执行迁移脚本（与 000001_init_schema.up.sql 一致，无外键约束）
mysql -u root -p"${MYSQL_ROOT_PASSWORD}" "${MYSQL_DATABASE}" << 'EOSQL'

-- =============================================
-- Migration: 000001_init_schema.up.sql
-- Description: 初始化所有表结构
-- =============================================

-- 管理员表
CREATE TABLE IF NOT EXISTS admins (
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

-- 租户表
CREATE TABLE IF NOT EXISTS tenants (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(50) UNIQUE COMMENT '租户编码',
    contact_email VARCHAR(100) COMMENT '联系邮箱',
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

-- 租户配额表
CREATE TABLE IF NOT EXISTS tenant_quotas (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    max_concurrent_tasks INT DEFAULT 5 COMMENT '最大并发任务数',
    max_daily_tasks INT DEFAULT 100 COMMENT '每日最大任务数',
    max_daily_size_gb DECIMAL(10,2) DEFAULT 50 COMMENT '每日最大导出量(GB)',
    max_retention_hours INT DEFAULT 720 COMMENT '最长文件保留时间(小时)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_tenant_id (tenant_id),
    UNIQUE KEY uk_tenant_id (tenant_id)
) ENGINE=InnoDB;

-- TiDB连接配置表
CREATE TABLE IF NOT EXISTS tidb_configs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL COMMENT '配置名称',
    host VARCHAR(255) NOT NULL,
    port INT DEFAULT 4000,
    username VARCHAR(100) NOT NULL,
    password_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    `database` VARCHAR(100),
    ssl_mode VARCHAR(20) DEFAULT 'disabled' COMMENT 'SSL模式: disabled/preferred/required/verify_identity',
    status TINYINT DEFAULT 1 COMMENT '1=启用 0=禁用',
    is_default TINYINT DEFAULT 0 COMMENT '是否为默认配置',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL COMMENT '软删除标记',
    INDEX idx_tenant_id (tenant_id),
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_tenant_default (tenant_id, is_default),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB;

-- S3配置表
CREATE TABLE IF NOT EXISTS s3_configs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL COMMENT '配置名称',
    endpoint VARCHAR(255) NOT NULL COMMENT 'endpoint, 如s3.amazonaws.com',
    access_key VARCHAR(255) NOT NULL,
    secret_key_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    bucket VARCHAR(100) NOT NULL,
    region VARCHAR(50),
    path_prefix VARCHAR(255) DEFAULT '' COMMENT '路径前缀',
    status TINYINT DEFAULT 1 COMMENT '1=启用 0=禁用',
    is_default TINYINT DEFAULT 0 COMMENT '是否为默认配置',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL COMMENT '软删除标记',
    INDEX idx_tenant_id (tenant_id),
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_tenant_default (tenant_id, is_default),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB;

-- 导出任务表
CREATE TABLE IF NOT EXISTS export_tasks (
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
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_tidb_config_id (tidb_config_id),
    INDEX idx_s3_config_id (s3_config_id),
    INDEX idx_tenant_status (tenant_id, status),
    INDEX idx_priority (priority),
    INDEX idx_created_at (created_at),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB;

-- 任务执行日志表
CREATE TABLE IF NOT EXISTS task_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL,
    log_level VARCHAR(10) COMMENT 'INFO/ERROR/WARN',
    message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_id (task_id),
    INDEX idx_task_created (task_id, created_at)
) ENGINE=InnoDB;

-- 审计日志表
CREATE TABLE IF NOT EXISTS audit_logs (
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

-- Dumpling参数模板表
CREATE TABLE IF NOT EXISTS dumpling_templates (
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
    INDEX idx_tenant_id (tenant_id),
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB;

EOSQL

echo "Tables created successfully!"

# 检查是否需要创建默认管理员
ADMIN_EXISTS=$(mysql -u root -p"${MYSQL_ROOT_PASSWORD}" "${MYSQL_DATABASE}" -sse "SELECT COUNT(*) FROM admins WHERE username='admin';")

if [ "$ADMIN_EXISTS" -eq 0 ]; then
    echo "Creating default admin user..."
    # 密码: admin123 (bcrypt hash generated by htpasswd)
    mysql -u root -p"${MYSQL_ROOT_PASSWORD}" "${MYSQL_DATABASE}" -e "
        INSERT INTO admins (username, password_hash, email, role, status) 
        VALUES ('admin', '\$2y\$10\$WdQsrT8PpgRYm1Q6zlzZTOtbu8LwcpTKMThRdyiWC5t6uk9oZ5zjC', 'admin@example.com', 'admin', 1);
    "
    echo "Default admin user created!"
    echo "  Username: admin"
    echo "  Password: admin123"
else
    echo "Admin user already exists, skipping..."
fi

echo "=========================================="
echo "Database initialization completed!"
echo "=========================================="
