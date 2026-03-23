-- =============================================
-- Migration: 000001_init_schema.down.sql
-- Description: 回滚所有表结构（按依赖关系逆序删除）
-- =============================================

DROP TABLE IF EXISTS dumpling_templates;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS task_logs;
DROP TABLE IF EXISTS export_tasks;
DROP TABLE IF EXISTS s3_configs;
DROP TABLE IF EXISTS tidb_configs;
DROP TABLE IF EXISTS tenant_quotas;
DROP TABLE IF EXISTS tenants;
DROP TABLE IF EXISTS admins;
