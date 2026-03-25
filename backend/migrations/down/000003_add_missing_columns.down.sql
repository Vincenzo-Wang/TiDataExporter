-- =============================================
-- Migration: 000003_add_missing_columns.down.sql
-- Description: 回滚添加的字段
-- =============================================

-- 租户表：移除 code 和 contact_email 字段
ALTER TABLE tenants 
    DROP COLUMN IF EXISTS contact_email,
    DROP COLUMN IF EXISTS code;

-- TiDB配置表：移除 ssl_mode 和 status 字段
ALTER TABLE tidb_configs 
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS ssl_mode;

-- S3配置表：移除 status 字段
ALTER TABLE s3_configs 
    DROP COLUMN IF EXISTS status;
