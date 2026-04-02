-- =============================================
-- Migration: 000001_init_schema.up.sql
-- Description: 初始化所有表结构（TiDB 兼容基线）
-- =============================================

CREATE TABLE IF NOT EXISTS admins (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL COMMENT 'bcrypt哈希',
    email VARCHAR(100) DEFAULT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'admin' COMMENT 'admin/operator',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用',
    last_login_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='管理员账号表';

CREATE TABLE IF NOT EXISTS tenants (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(50) DEFAULT NULL COMMENT '租户编码',
    contact_email VARCHAR(100) DEFAULT NULL COMMENT '联系邮箱',
    api_key VARCHAR(64) UNIQUE NOT NULL,
    api_secret_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL COMMENT '软删除标记',
    UNIQUE KEY uk_tenants_code (code),
    INDEX idx_status (status),
    INDEX idx_api_key (api_key),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='租户信息表';

CREATE TABLE IF NOT EXISTS tenant_quotas (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    max_concurrent_tasks INT NOT NULL DEFAULT 5 COMMENT '最大并发任务数',
    max_daily_tasks INT NOT NULL DEFAULT 100 COMMENT '每日最大任务数',
    max_daily_size_gb DECIMAL(10,2) NOT NULL DEFAULT 50 COMMENT '每日最大导出量(GB)',
    max_retention_hours INT NOT NULL DEFAULT 720 COMMENT '最长文件保留时间(小时)',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_tenant_id (tenant_id),
    INDEX idx_tenant_id (tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='租户配额表';

CREATE TABLE IF NOT EXISTS tidb_configs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL COMMENT '配置名称',
    host VARCHAR(255) NOT NULL COMMENT 'TiDB 地址',
    port INT NOT NULL DEFAULT 4000 COMMENT 'TiDB 端口',
    username VARCHAR(100) NOT NULL COMMENT 'TiDB 用户名',
    password_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    `database` VARCHAR(100) DEFAULT NULL COMMENT '默认数据库',
    ssl_mode VARCHAR(20) NOT NULL DEFAULT 'disabled' COMMENT 'SSL模式: disabled/preferred/required/verify_identity',
    ssl_ca VARCHAR(255) DEFAULT NULL COMMENT 'TLS CA 证书路径',
    ssl_cert VARCHAR(255) DEFAULT NULL COMMENT 'TLS 客户端证书路径',
    ssl_key VARCHAR(255) DEFAULT NULL COMMENT 'TLS 客户端私钥路径',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用',
    is_default TINYINT NOT NULL DEFAULT 0 COMMENT '是否为默认配置',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL COMMENT '软删除标记',
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_tenant_default (tenant_id, is_default),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='租户 TiDB 连接配置表';

CREATE TABLE IF NOT EXISTS s3_configs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL COMMENT '配置名称',
    provider VARCHAR(20) NOT NULL DEFAULT 'aws' COMMENT '存储厂商: aws, aliyun',
    endpoint VARCHAR(255) NOT NULL COMMENT 'endpoint, 如 s3.amazonaws.com',
    access_key VARCHAR(255) NOT NULL,
    secret_key_encrypted TEXT NOT NULL COMMENT 'AES-256加密存储',
    bucket VARCHAR(100) NOT NULL,
    region VARCHAR(50) DEFAULT NULL,
    path_prefix VARCHAR(255) NOT NULL DEFAULT '' COMMENT '路径前缀',
    status TINYINT NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用',
    is_default TINYINT NOT NULL DEFAULT 0 COMMENT '是否为默认配置',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL COMMENT '软删除标记',
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_tenant_default (tenant_id, is_default),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='S3/OSS 存储配置表';

CREATE TABLE IF NOT EXISTS export_tasks (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    task_name VARCHAR(255) DEFAULT NULL,
    tidb_config_id BIGINT NOT NULL,
    s3_config_id BIGINT NOT NULL,
    sql_text TEXT NOT NULL,
    filetype VARCHAR(10) NOT NULL DEFAULT 'sql' COMMENT 'sql/csv',
    compress VARCHAR(20) DEFAULT NULL COMMENT 'gzip/snappy/zstd',
    retention_hours INT NOT NULL DEFAULT 168 COMMENT '文件保留时间（小时）',
    priority INT NOT NULL DEFAULT 5 COMMENT '任务优先级 1-10, 10最高',
    status VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT 'pending/running/success/failed/canceled/expired',
    file_url VARCHAR(500) DEFAULT NULL COMMENT 'S3下载地址',
    file_size BIGINT DEFAULT NULL COMMENT '文件大小（字节）',
    row_count BIGINT DEFAULT NULL COMMENT '导出行数',
    error_message TEXT DEFAULT NULL,
    cancel_reason TEXT DEFAULT NULL COMMENT '取消原因',
    retry_count INT NOT NULL DEFAULT 0 COMMENT '重试次数',
    max_retries INT NOT NULL DEFAULT 3 COMMENT '最大重试次数',
    started_at TIMESTAMP NULL DEFAULT NULL,
    completed_at TIMESTAMP NULL DEFAULT NULL,
    expires_at TIMESTAMP NULL DEFAULT NULL COMMENT '文件过期时间',
    canceled_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_tidb_config_id (tidb_config_id),
    INDEX idx_s3_config_id (s3_config_id),
    INDEX idx_tenant_status (tenant_id, status),
    INDEX idx_priority (priority),
    INDEX idx_created_at (created_at),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='数据导出任务表';

CREATE TABLE IF NOT EXISTS task_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL,
    log_level VARCHAR(10) DEFAULT NULL COMMENT 'INFO/ERROR/WARN',
    message TEXT DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_id (task_id),
    INDEX idx_task_created (task_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='任务执行日志表';

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT DEFAULT NULL,
    admin_id BIGINT DEFAULT NULL,
    action VARCHAR(50) NOT NULL COMMENT '操作类型',
    resource_type VARCHAR(50) DEFAULT NULL COMMENT '资源类型',
    resource_id BIGINT DEFAULT NULL COMMENT '资源ID',
    request_ip VARCHAR(45) DEFAULT NULL,
    user_agent VARCHAR(255) DEFAULT NULL,
    request_data TEXT DEFAULT NULL COMMENT '请求数据(脱敏)',
    result VARCHAR(20) DEFAULT NULL COMMENT 'success/failed',
    error_message TEXT DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tenant_created (tenant_id, created_at),
    INDEX idx_admin_created (admin_id, created_at),
    INDEX idx_resource (resource_type, resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='管理操作审计日志表';

CREATE TABLE IF NOT EXISTS dumpling_templates (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    threads INT NOT NULL DEFAULT 4,
    rows_per_file INT NOT NULL DEFAULT 0 COMMENT '表内并发，0=关闭',
    file_size VARCHAR(20) NOT NULL DEFAULT '256MiB',
    consistency VARCHAR(20) NOT NULL DEFAULT 'auto',
    filter_rule JSON DEFAULT NULL COMMENT '过滤规则，JSON格式',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL COMMENT '软删除标记',
    UNIQUE KEY uk_tenant_name (tenant_id, name),
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci /*T![auto_id_cache] AUTO_ID_CACHE=1 */ COMMENT='Dumpling 参数模板表';
