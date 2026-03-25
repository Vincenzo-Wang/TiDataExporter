-- =============================================
-- Migration: 000003_add_missing_columns.up.sql
-- Description: 添加缺失的字段（旧数据库升级）
-- 注意：如果字段已存在会报错，可忽略错误继续执行
-- =============================================

-- 租户表：添加 code 和 contact_email 字段
ALTER TABLE tenants ADD COLUMN code VARCHAR(50) UNIQUE COMMENT '租户编码' AFTER name;
ALTER TABLE tenants ADD COLUMN contact_email VARCHAR(100) COMMENT '联系邮箱' AFTER code;

-- TiDB配置表：添加 ssl_mode 和 status 字段
ALTER TABLE tidb_configs ADD COLUMN ssl_mode VARCHAR(20) DEFAULT 'disabled' COMMENT 'SSL模式: disabled/preferred/required/verify_identity' AFTER `database`;
ALTER TABLE tidb_configs ADD COLUMN status TINYINT DEFAULT 1 COMMENT '1=启用 0=禁用' AFTER ssl_mode;

-- S3配置表：添加 status 字段
ALTER TABLE s3_configs ADD COLUMN status TINYINT DEFAULT 1 COMMENT '1=启用 0=禁用' AFTER path_prefix;
