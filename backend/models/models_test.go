package models

import "testing"

func TestAdminTableName(t *testing.T) {
	a := Admin{}
	if got := a.TableName(); got != "admins" {
		t.Errorf("Admin.TableName() = %q, want %q", got, "admins")
	}
}

func TestTenantTableName(t *testing.T) {
	tn := Tenant{}
	if got := tn.TableName(); got != "tenants" {
		t.Errorf("Tenant.TableName() = %q, want %q", got, "tenants")
	}

	tq := TenantQuota{}
	if got := tq.TableName(); got != "tenant_quotas" {
		t.Errorf("TenantQuota.TableName() = %q, want %q", got, "tenant_quotas")
	}
}

func TestConfigTableNames(t *testing.T) {
	tc := TiDBConfig{}
	if got := tc.TableName(); got != "tidb_configs" {
		t.Errorf("TiDBConfig.TableName() = %q, want %q", got, "tidb_configs")
	}

	sc := S3Config{}
	if got := sc.TableName(); got != "s3_configs" {
		t.Errorf("S3Config.TableName() = %q, want %q", got, "s3_configs")
	}

	dt := DumplingTemplate{}
	if got := dt.TableName(); got != "dumpling_templates" {
		t.Errorf("DumplingTemplate.TableName() = %q, want %q", got, "dumpling_templates")
	}
}

func TestTaskTableNames(t *testing.T) {
	et := ExportTask{}
	if got := et.TableName(); got != "export_tasks" {
		t.Errorf("ExportTask.TableName() = %q, want %q", got, "export_tasks")
	}

	tl := TaskLog{}
	if got := tl.TableName(); got != "task_logs" {
		t.Errorf("TaskLog.TableName() = %q, want %q", got, "task_logs")
	}

	al := AuditLog{}
	if got := al.TableName(); got != "audit_logs" {
		t.Errorf("AuditLog.TableName() = %q, want %q", got, "audit_logs")
	}
}
