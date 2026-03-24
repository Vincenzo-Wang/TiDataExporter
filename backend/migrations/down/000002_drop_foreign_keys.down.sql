-- =============================================
-- Migration: 000002_drop_foreign_keys.down.sql
-- Description: 回滚 - 重新添加外键约束
-- =============================================

-- 重新添加 tenant_quotas 表的外键
ALTER TABLE tenant_quotas ADD CONSTRAINT tenant_quotas_ibfk_1 FOREIGN KEY (tenant_id) REFERENCES tenants(id);

-- 重新添加 tidb_configs 表的外键
ALTER TABLE tidb_configs ADD CONSTRAINT tidb_configs_ibfk_1 FOREIGN KEY (tenant_id) REFERENCES tenants(id);

-- 重新添加 s3_configs 表的外键
ALTER TABLE s3_configs ADD CONSTRAINT s3_configs_ibfk_1 FOREIGN KEY (tenant_id) REFERENCES tenants(id);

-- 重新添加 export_tasks 表的外键
ALTER TABLE export_tasks ADD CONSTRAINT export_tasks_ibfk_1 FOREIGN KEY (tenant_id) REFERENCES tenants(id);
ALTER TABLE export_tasks ADD CONSTRAINT export_tasks_ibfk_2 FOREIGN KEY (tidb_config_id) REFERENCES tidb_configs(id);
ALTER TABLE export_tasks ADD CONSTRAINT export_tasks_ibfk_3 FOREIGN KEY (s3_config_id) REFERENCES s3_configs(id);

-- 重新添加 task_logs 表的外键
ALTER TABLE task_logs ADD CONSTRAINT task_logs_ibfk_1 FOREIGN KEY (task_id) REFERENCES export_tasks(id) ON DELETE CASCADE;

-- 重新添加 dumpling_templates 表的外键
ALTER TABLE dumpling_templates ADD CONSTRAINT dumpling_templates_ibfk_1 FOREIGN KEY (tenant_id) REFERENCES tenants(id);
