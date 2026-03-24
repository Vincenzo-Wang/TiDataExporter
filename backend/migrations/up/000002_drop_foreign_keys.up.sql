-- =============================================
-- Migration: 000002_drop_foreign_keys.up.sql
-- Description: 删除所有外键约束
-- =============================================

-- 删除 tenant_quotas 表的外键
ALTER TABLE tenant_quotas DROP FOREIGN KEY IF EXISTS tenant_quotas_ibfk_1;

-- 删除 tidb_configs 表的外键
ALTER TABLE tidb_configs DROP FOREIGN KEY IF EXISTS tidb_configs_ibfk_1;

-- 删除 s3_configs 表的外键
ALTER TABLE s3_configs DROP FOREIGN KEY IF EXISTS s3_configs_ibfk_1;

-- 删除 export_tasks 表的外键
ALTER TABLE export_tasks DROP FOREIGN KEY IF EXISTS export_tasks_ibfk_1;
ALTER TABLE export_tasks DROP FOREIGN KEY IF EXISTS export_tasks_ibfk_2;
ALTER TABLE export_tasks DROP FOREIGN KEY IF EXISTS export_tasks_ibfk_3;

-- 删除 task_logs 表的外键
ALTER TABLE task_logs DROP FOREIGN KEY IF EXISTS task_logs_ibfk_1;

-- 删除 dumpling_templates 表的外键
ALTER TABLE dumpling_templates DROP FOREIGN KEY IF EXISTS dumpling_templates_ibfk_1;

-- 为关联字段添加索引（如果不存在）
-- 注意：这些索引在 init_schema.up.sql 中已经定义，这里只是确保
-- 如果索引已存在会报错，可以忽略或使用存储过程检查后创建
