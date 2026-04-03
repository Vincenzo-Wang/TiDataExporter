-- =============================================
-- Migration: 000002_add_biz_name_to_export_tasks.up.sql
-- Description: export_tasks 新增业务命名字段 biz_name
-- =============================================

ALTER TABLE export_tasks
    ADD COLUMN biz_name VARCHAR(64) NOT NULL DEFAULT '' AFTER task_name;
