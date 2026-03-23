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
  api_secret_encrypted: string;
  quota_daily: number;
  quota_used_today: number;
  quota_monthly: number;
  quota_used_month: number;
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
  password_encrypted: string;
  database: string;
  ssl_mode: string;
  status: number;
  created_at: string;
}

export interface S3Config {
  id: number;
  tenant_id: number;
  name: string;
  endpoint: string;
  bucket: string;
  region: string;
  access_key_id_encrypted: string;
  secret_access_key_encrypted: string;
  path_prefix: string;
  status: number;
  created_at: string;
}

export interface ExportTask {
  id: number;
  tenant_id: number;
  task_no: string;
  tidb_config_name: string;
  s3_config_name: string;
  sql_text: string;
  filetype: string;
  compress: string;
  s3_path: string;
  file_size: number;
  row_count: number;
  status: number;
  progress: number;
  error_message: string;
  retry_count: number;
  priority: number;
  retention_hours: number;
  created_at: string;
  started_at: string;
  completed_at: string;
  expired_at: string;
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
