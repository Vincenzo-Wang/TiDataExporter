import { useEffect, useState } from 'react';
import { Row, Col, Card, Statistic, Table, Tag, Progress, Typography } from 'antd';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  DatabaseOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import api from '@/services/api';
import type { ApiResponse, ExportTask, TaskStatus, TaskStatistics } from '@/types';

const { Title } = Typography;

const taskStatusMap: Record<TaskStatus, { color: string; text: string }> = {
  pending: { color: 'default', text: '待处理' },
  running: { color: 'processing', text: '运行中' },
  success: { color: 'success', text: '成功' },
  failed: { color: 'error', text: '失败' },
  canceled: { color: 'warning', text: '已取消' },
  expired: { color: 'default', text: '已过期' },
};

const formatBytes = (size: number) => {
  if (!size) return '-';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const digits = unitIndex === 0 ? 0 : 2;
  return `${value.toFixed(digits)} ${units[unitIndex]}`;
};

const formatRows = (rows: number) => {
  if (!rows) return '-';
  if (rows < 10000) return `${rows.toLocaleString()} 行`;
  if (rows < 100000000) return `${(rows / 10000).toFixed(2)} 万行`;
  if (rows < 1000000000000) return `${(rows / 100000000).toFixed(2)} 亿行`;
  return `${(rows / 1000000000000).toFixed(2)} 万亿行`;
};

export default function Dashboard() {
  const [statistics, setStatistics] = useState<TaskStatistics | null>(null);
  const [recentTasks, setRecentTasks] = useState<ExportTask[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    fetchDashboardData();
  }, []);

  const fetchDashboardData = async () => {
    setLoading(true);
    try {
      const [statsRes, tasksRes] = await Promise.all([
        api.get<ApiResponse<TaskStatistics>>('/admin/statistics/overview'),
        api.get<ApiResponse<{ items: ExportTask[] }>>('/admin/tasks?page=1&page_size=10'),
      ]);
      
      if (statsRes.data.code === 0) {
        setStatistics(statsRes.data.data);
      }
      if (tasksRes.data.code === 0) {
        setRecentTasks(tasksRes.data.data.items);
      }
    } catch (error) {
      console.error('Failed to fetch dashboard data:', error);
    } finally {
      setLoading(false);
    }
  };

  const columns: ColumnsType<ExportTask> = [
    {
      title: '任务编号',
      dataIndex: 'task_id',
      key: 'task_id',
      width: 100,
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
      title: '文件大小',
      dataIndex: 'file_size',
      key: 'file_size',
      width: 100,
      render: (size: number) => formatBytes(size),
    },
    {
      title: '行数',
      dataIndex: 'row_count',
      key: 'row_count',
      width: 100,
      render: (count: number) => count?.toLocaleString() || '-',
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (time: string) => time ? new Date(time).toLocaleString('zh-CN') : '-',
    },
  ];

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>
        仪表盘
      </Title>
      
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="待处理任务"
              value={statistics?.pending_tasks || 0}
              prefix={<ClockCircleOutlined />}
              valueStyle={{ color: '#faad14' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="运行中任务"
              value={statistics?.running_tasks || 0}
              prefix={<SyncOutlined spin />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="成功任务"
              value={statistics?.success_tasks || 0}
              prefix={<CheckCircleOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="失败任务"
              value={statistics?.failed_tasks || 0}
              prefix={<CloseCircleOutlined />}
              valueStyle={{ color: '#ff4d4f' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="总任务数"
              value={statistics?.total_tasks || 0}
              prefix={<DatabaseOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="总数据行数"
              value={formatRows(statistics?.total_rows || 0)}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="总数据大小"
              value={formatBytes(statistics?.total_size || 0)}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="平均执行时间"
              value={statistics?.avg_duration || 0}
              suffix="秒"
            />
          </Card>
        </Col>
      </Row>

      <Card title="最近任务" style={{ marginTop: 24 }} loading={loading}>
        <Table
          columns={columns}
          dataSource={recentTasks}
          rowKey="task_id"
          pagination={false}
          size="small"
        />
      </Card>
    </div>
  );
}
