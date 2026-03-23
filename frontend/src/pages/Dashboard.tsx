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
import type { ApiResponse, ExportTask, TaskStatistics } from '@/types';

const { Title } = Typography;

const taskStatusMap: Record<number, { color: string; text: string }> = {
  0: { color: 'default', text: '待处理' },
  1: { color: 'processing', text: '运行中' },
  2: { color: 'success', text: '成功' },
  3: { color: 'error', text: '失败' },
  4: { color: 'warning', text: '已取消' },
  5: { color: 'default', text: '超时' },
  6: { color: 'processing', text: '重试中' },
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
      dataIndex: 'task_no',
      key: 'task_no',
      width: 140,
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
      render: (status: number) => {
        const { color, text } = taskStatusMap[status] || { color: 'default', text: '未知' };
        return <Tag color={color}>{text}</Tag>;
      },
    },
    {
      title: '进度',
      dataIndex: 'progress',
      key: 'progress',
      width: 120,
      render: (progress: number) => <Progress percent={progress} size="small" />,
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
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (time: string) => new Date(time).toLocaleString('zh-CN'),
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
              value={statistics?.total_rows || 0}
              suffix="行"
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card loading={loading}>
            <Statistic
              title="总数据大小"
              value={statistics?.total_size || 0}
              suffix="bytes"
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
          rowKey="id"
          pagination={false}
          size="small"
        />
      </Card>
    </div>
  );
}
