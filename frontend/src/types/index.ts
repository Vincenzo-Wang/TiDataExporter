export interface Admin {
  id: number;
  username: string;
  email: string;
  role: string;
  last_login_at: string;
  created_at: string;
}

export interface Tenant {
  id: number;
  name: string;
  code: string;
  contact_email: string;
  api_key: string;
  quota_daily: number;
  quota_used_today: number;
  quota_monthly: number;
  quota_used_month: number;
  max_concurrent: number;
  max_size_gb: number;
  retention_hours: number;
  status: number;
  created_at: string;
  updated_at: string;
}

export interface TiDBConfig {
  id: number;
  tenant_id: number;
  name: string;
  host: string;
  port: number;
  username: string;
  database: string;
  ssl_mode: string;
  status: number;
  is_default: number;
  created_at: string;
  updated_at: string;
}

export interface S3Config {
  id: number;
  tenant_id: number;
  name: string;
  provider: 'aws' | 'aliyun';
  endpoint: string;
  bucket: string;
  region: string;
  path_prefix: string;
  status: number;
  is_default: number;
  created_at: string;
  updated_at: string;
}

export type TaskStatus = 'pending' | 'running' | 'success' | 'failed' | 'canceled' | 'expired';

export interface ExportTaskFile {
  index?: number;
  name?: string;
  raw_name?: string;
  path: string;
  url?: string;
  size: number;
}

export interface ExportTask {
  id: number;
  task_id: number;
  tenant_id: number;
  tenant_name: string;
  task_name: string;
  biz_name?: string;
  tidb_config_id: number;
  tidb_config_name: string;
  s3_config_id: number;
  s3_config_name: string;
  sql_text: string;
  filetype: string;
  compress: string;
  file_url: string;
  files?: ExportTaskFile[];
  file_count?: number;
  file_size: number;
  row_count: number;
  status: TaskStatus;
  progress: number;
  error_message: string;
  cancel_reason: string;
  retry_count: number;
  max_retries: number;
  priority: number;
  retention_hours: number;
  started_at: string;
  completed_at: string;
  expires_at: string;
  canceled_at: string;
  created_at: string;
  updated_at: string;
}

export interface TaskLog {
  id: number;
  task_id: number;
  log_level: string;
  message: string;
  created_at: string;
}

export interface TaskStatistics {
  total_tasks: number;
  pending_tasks: number;
  running_tasks: number;
  success_tasks: number;
  failed_tasks: number;
  canceled_tasks: number;
  total_rows: number;
  total_size: number;
  avg_duration: number;
}

export interface DailyStatistics {
  date: string;
  task_count: number;
  success_count: number;
  failed_count: number;
  total_rows: number;
  total_size: number;
}

export interface TenantStatistics {
  tenant_id: number;
  tenant_name: string;
  task_count: number;
  success_count: number;
  failed_count: number;
  total_size: number;
  success_rate: number;
  failure_rate: number;
}

export interface ApiResponse<T = unknown> {
  code: number;
  message: string;
  data: T;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface LoginResponse {
  token: string;
  expires_in: number;
  user: Admin;
}
