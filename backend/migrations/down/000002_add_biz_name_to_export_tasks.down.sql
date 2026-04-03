-- =============================================
-- Migration: 000002_add_biz_name_to_export_tasks.down.sql
-- Description: 回滚删除 export_tasks.biz_name 字段
-- =============================================

ALTER TABLE export_tasks
    DROP COLUMN biz_name;
