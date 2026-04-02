-- =============================================
-- Migration: 000004_add_s3_provider.up.sql
-- Description: 补齐 S3/OSS 存储厂商字段（幂等升级）
-- =============================================

ALTER TABLE s3_configs ADD COLUMN IF NOT EXISTS provider VARCHAR(20) NOT NULL DEFAULT 'aws' COMMENT '存储厂商: aws, aliyun';
UPDATE s3_configs SET provider = 'aws' WHERE provider IS NULL OR provider = '';
