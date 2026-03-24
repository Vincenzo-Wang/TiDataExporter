import { useEffect, useState, useCallback } from 'react';
import { Table, Card, Tag, Progress, Button, Space, Input, Select, DatePicker, Modal, message } from 'antd';
import { SearchOutlined, ReloadOutlined, EyeOutlined, StopOutlined, RedoOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { useNavigate } from 'react-router-dom';
import api from '@/services/api';
import type { ApiResponse, ExportTask, TaskStatus, PaginatedResponse } from '@/types';

const { RangePicker } = DatePicker;

const taskStatusMap: Record<TaskStatus, { color: string; text: string }> = {
  pending: { color: 'default', text: '待处理' },
  running: { color: 'processing', text: '运行中' },
  success: { color: 'success', text: '成功' },
  failed: { color: 'error', text: '失败' },
  canceled: { color: 'warning', text: '已取消' },
  expired: { color: 'default', text: '已过期' },
};

const fileTypeMap: Record<string, string> = {
  csv: 'CSV',
  sql: 'SQL',
  parquet: 'Parquet',
};

const compressMap: Record<string, string> = {
  none: '无',
  gzip: 'GZIP',
  gz: 'GZIP',
  zstd: 'ZSTD',
  snappy: 'Snappy',
};

export default function Tasks() {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [tasks, setTasks] = useState<ExportTask[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [searchText, setSearchText] = useState('');
  const [statusFilter, setStatusFilter] = useState<TaskStatus | undefined>();
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs] | null>(null);

  const fetchTasks = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        page_size: String(pageSize),
      });
      if (searchText) params.append('search', searchText);
      if (statusFilter !== undefined) params.append('status', statusFilter);
      if (dateRange) {
        params.append('start_date', dateRange[0].format('YYYY-MM-DD'));
        params.append('end_date', dateRange[1].format('YYYY-MM-DD'));
      }

      const response = await api.get<ApiResponse<PaginatedResponse<ExportTask>>>(`/admin/tasks?${params}`);
      if (response.data.code === 0) {
        setTasks(response.data.data.items);
        setTotal(response.data.data.total);
      }
    } catch (error) {
      console.error('Failed to fetch tasks:', error);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, searchText, statusFilter, dateRange]);

  useEffect(() => {
    fetchTasks();
  }, [fetchTasks]);

  const handleCancel = async (taskId: number) => {
    Modal.confirm({
      title: '确认取消',
      content: '确定要取消此任务吗？',
      onOk: async () => {
        try {
          const response = await api.post<ApiResponse<unknown>>(`/admin/tasks/${taskId}/cancel`);
          if (response.data.code === 0) {
            message.success('任务已取消');
            fetchTasks();
          } else {
            message.error(response.data.message || '操作失败');
          }
        } catch {
          message.error('操作失败');
        }
      },
    });
  };

  const handleRetry = async (taskId: number) => {
    try {
      const response = await api.post<ApiResponse<unknown>>(`/admin/tasks/${taskId}/retry`);
      if (response.data.code === 0) {
        message.success('任务已重新提交');
        fetchTasks();
      } else {
        message.error(response.data.message || '操作失败');
      }
    } catch {
      message.error('操作失败');
    }
  };

  const columns: ColumnsType<ExportTask> = [
    {
      title: '任务ID',
      dataIndex: 'task_id',
      key: 'task_id',
      width: 80,
      render: (text, record) => (
        <Button type="link" onClick={() => navigate(`/tasks/${record.task_id}`)}>
          {text}
        </Button>
      ),
    },
    {
      title: '任务名称',
      dataIndex: 'task_name',
      key: 'task_name',
      width: 150,
      ellipsis: true,
    },
    {
      title: '租户',
      dataIndex: 'tenant_name',
      key: 'tenant_name',
      width: 100,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: TaskStatus) => {
        const info = taskStatusMap[status] || { color: 'default', text: status };
        return <Tag color={info.color}>{info.text}</Tag>;
      },
    },
    {
      title: '进度',
      dataIndex: 'progress',
      key: 'progress',
      width: 120,
      render: (progress: number) => <Progress percent={progress || 0} size="small" />,
    },
    {
      title: '文件类型',
      dataIndex: 'filetype',
      key: 'filetype',
      width: 80,
      render: (type: string) => fileTypeMap[type] || type,
    },
    {
      title: '压缩',
      dataIndex: 'compress',
      key: 'compress',
      width: 80,
      render: (compress: string) => compressMap[compress] || compress || '无',
    },
    {
      title: '文件大小',
      dataIndex: 'file_size',
      key: 'file_size',
      width: 100,
      render: (size: number) => {
        if (!size) return '-';
        if (size < 1024) return `${size} B`;
        if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`;
        if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(2)} MB`;
        return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
      },
    },
    {
      title: '行数',
      dataIndex: 'row_count',
      key: 'row_count',
      width: 100,
      render: (count: number) => count?.toLocaleString() || '-',
    },
    {
      title: '重试',
      dataIndex: 'retry_count',
      key: 'retry_count',
      width: 60,
      render: (count: number, record) => `${count}/${record.max_retries || 3}`,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (time: string) => time ? new Date(time).toLocaleString('zh-CN') : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_, record) => (
        <Space size="small">
          <Button
            type="link"
            size="small"
            icon={<EyeOutlined />}
            onClick={() => navigate(`/tasks/${record.task_id}`)}
          />
          {(record.status === 'pending' || record.status === 'running') && (
            <Button
              type="link"
              size="small"
              danger
              icon={<StopOutlined />}
              onClick={() => handleCancel(record.task_id)}
            />
          )}
          {record.status === 'failed' && (
            <Button
              type="link"
              size="small"
              icon={<RedoOutlined />}
              onClick={() => handleRetry(record.task_id)}
            />
          )}
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Card
        title="任务管理"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchTasks}>
              刷新
            </Button>
          </Space>
        }
      >
        <Space style={{ marginBottom: 16 }} wrap>
          <Input
            placeholder="搜索任务名称"
            prefix={<SearchOutlined />}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
            allowClear
          />
          <Select
            placeholder="状态筛选"
            style={{ width: 120 }}
            allowClear
            value={statusFilter}
            onChange={setStatusFilter}
            options={Object.entries(taskStatusMap).map(([key, value]) => ({
              value: key as TaskStatus,
              label: value.text,
            }))}
          />
          <RangePicker
            value={dateRange}
            onChange={(dates) => setDateRange(dates as [dayjs.Dayjs, dayjs.Dayjs] | null)}
          />
        </Space>

        <Table
          columns={columns}
          dataSource={tasks}
          rowKey="task_id"
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (p, ps) => {
              setPage(p);
              setPageSize(ps);
            },
          }}
        />
      </Card>
    </div>
  );
}
