-- =============================================
-- Migration: 000003_add_missing_columns.up.sql
-- Description: 添加缺失字段（幂等升级）
-- =============================================

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS code VARCHAR(50) DEFAULT NULL COMMENT '租户编码';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS contact_email VARCHAR(100) DEFAULT NULL COMMENT '联系邮箱';

ALTER TABLE tidb_configs ADD COLUMN IF NOT EXISTS ssl_mode VARCHAR(20) NOT NULL DEFAULT 'disabled' COMMENT 'SSL模式: disabled/preferred/required/verify_identity';
ALTER TABLE tidb_configs ADD COLUMN IF NOT EXISTS ssl_ca VARCHAR(255) DEFAULT NULL COMMENT 'TLS CA 证书路径';
ALTER TABLE tidb_configs ADD COLUMN IF NOT EXISTS ssl_cert VARCHAR(255) DEFAULT NULL COMMENT 'TLS 客户端证书路径';
ALTER TABLE tidb_configs ADD COLUMN IF NOT EXISTS ssl_key VARCHAR(255) DEFAULT NULL COMMENT 'TLS 客户端私钥路径';
ALTER TABLE tidb_configs ADD COLUMN IF NOT EXISTS status TINYINT NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用';

ALTER TABLE s3_configs ADD COLUMN IF NOT EXISTS status TINYINT NOT NULL DEFAULT 1 COMMENT '1=启用 0=禁用';
