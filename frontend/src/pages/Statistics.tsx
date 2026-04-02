import { useEffect, useMemo, useState } from 'react';
import { Card, Row, Col, DatePicker, Typography, Select, Table, message, Space } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { Line, Pie, Column } from '@ant-design/charts';
import dayjs from 'dayjs';
import api from '@/services/api';
import type { ApiResponse, DailyStatistics, PaginatedResponse, TaskStatistics, Tenant, TenantStatistics } from '@/types';

const { Title } = Typography;
const { RangePicker } = DatePicker;

export default function Statistics() {
  const [loading, setLoading] = useState(false);
  const [dailyStats, setDailyStats] = useState<DailyStatistics[]>([]);
  const [overviewStats, setOverviewStats] = useState<TaskStatistics | null>(null);
  const [tenantStats, setTenantStats] = useState<TenantStatistics[]>([]);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [selectedTenantId, setSelectedTenantId] = useState<number | undefined>();
  const [tenantSortBy, setTenantSortBy] = useState<'task_count' | 'failure_rate'>('task_count');
  const [tenantTopN, setTenantTopN] = useState<number>(10);
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>([
    dayjs().subtract(30, 'day'),
    dayjs(),
  ]);

  useEffect(() => {
    fetchTenants();
  }, []);

  useEffect(() => {
    fetchStatistics();
  }, [dateRange, selectedTenantId, tenantSortBy, tenantTopN]);

  const fetchTenants = async () => {
    try {
      const response = await api.get<ApiResponse<PaginatedResponse<Tenant>>>('/admin/tenants?page=1&page_size=200');
      if (response.data.code === 0) {
        setTenants(response.data.data.items || []);
      }
    } catch {
      message.error('加载租户列表失败');
    }
  };

  const fetchStatistics = async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        start_date: dateRange[0].format('YYYY-MM-DD'),
        end_date: dateRange[1].format('YYYY-MM-DD'),
      });
      if (selectedTenantId) {
        params.set('tenant_id', String(selectedTenantId));
      }

      const tenantParams = new URLSearchParams(params.toString());
      tenantParams.set('sort_by', tenantSortBy);
      tenantParams.set('order', 'desc');
      tenantParams.set('limit', String(tenantTopN));

      const [dailyRes, overviewRes, tenantRes] = await Promise.all([
        api.get<ApiResponse<DailyStatistics[]>>(`/admin/statistics/daily?${params.toString()}`),
        api.get<ApiResponse<TaskStatistics>>(`/admin/statistics/overview?${params.toString()}`),
        api.get<ApiResponse<TenantStatistics[]>>(`/admin/statistics/tenants?${tenantParams.toString()}`),
      ]);

      if (dailyRes.data.code === 0) {
        setDailyStats(dailyRes.data.data || []);
      }
      if (overviewRes.data.code === 0) {
        setOverviewStats(overviewRes.data.data);
      }
      if (tenantRes.data.code === 0) {
        setTenantStats(tenantRes.data.data || []);
      }
    } catch {
      message.error('加载统计数据失败');
    } finally {
      setLoading(false);
    }
  };

  const taskTrendConfig = {
    data: dailyStats,
    xField: 'date',
    yField: 'task_count',
    smooth: true,
  };

  const taskData = dailyStats.flatMap((item) => [
    { date: item.date, type: '成功', count: item.success_count },
    { date: item.date, type: '失败', count: item.failed_count },
  ]);

  const successFailConfig = {
    data: taskData,
    xField: 'date',
    yField: 'count',
    seriesField: 'type',
    isGroup: true,
  };

  const statusPieData = useMemo(
    () =>
      (overviewStats
        ? [
            { type: '待处理', value: overviewStats.pending_tasks },
            { type: '运行中', value: overviewStats.running_tasks },
            { type: '成功', value: overviewStats.success_tasks },
            { type: '失败', value: overviewStats.failed_tasks },
            { type: '已取消', value: overviewStats.canceled_tasks },
          ]
        : [])
        .filter((item) => item.value > 0),
    [overviewStats]
  );

  const statusPieConfig = {
    data: statusPieData,
    angleField: 'value',
    colorField: 'type',
    radius: 0.8,
    innerRadius: 0.6,
    legend: {
      position: 'bottom' as const,
    },
  };

  const dataSizeData = dailyStats.map((item) => ({
    date: item.date,
    size: Number((item.total_size / 1024 / 1024).toFixed(2)),
  }));

  const dataSizeConfig = {
    data: dataSizeData,
    xField: 'date',
    yField: 'size',
  };

  const tenantSortOptions = [
    { label: '任务数 TopN', value: 'task_count' },
    { label: '失败率 TopN', value: 'failure_rate' },
  ] as const;

  const tenantChartYField = tenantSortBy === 'failure_rate' ? 'failure_rate' : 'task_count';
  const tenantChartTitle = tenantSortBy === 'failure_rate' ? `失败率 Top${tenantTopN}` : `任务数 Top${tenantTopN}`;

  const tenantTaskConfig = {
    data: tenantStats,
    xField: 'tenant_name',
    yField: tenantChartYField,
    label: { position: 'top' as const },
  };

  const tenantColumns: ColumnsType<TenantStatistics> = [
    { title: '租户', dataIndex: 'tenant_name', key: 'tenant_name' },
    { title: '任务总数', dataIndex: 'task_count', key: 'task_count' },
    { title: '成功数', dataIndex: 'success_count', key: 'success_count' },
    { title: '失败数', dataIndex: 'failed_count', key: 'failed_count' },
    { title: '成功率', dataIndex: 'success_rate', key: 'success_rate', render: (v) => `${v}%` },
    { title: '失败率', dataIndex: 'failure_rate', key: 'failure_rate', render: (v) => `${v}%` },
    {
      title: '数据量(MB)',
      dataIndex: 'total_size',
      key: 'total_size',
      render: (v) => Number((v / 1024 / 1024).toFixed(2)),
    },
  ];

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>
        统计报表
      </Title>

      <Card style={{ marginBottom: 16 }}>
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12} lg={10}>
            <RangePicker
              value={dateRange}
              onChange={(dates) => dates && setDateRange(dates as [dayjs.Dayjs, dayjs.Dayjs])}
            />
          </Col>
          <Col xs={24} md={12} lg={8}>
            <Select
              allowClear
              placeholder="按租户筛选（可选）"
              style={{ width: '100%' }}
              value={selectedTenantId}
              onChange={(value) => setSelectedTenantId(value)}
              options={tenants.map((t) => ({ label: `${t.name} (${t.id})`, value: t.id }))}
            />
          </Col>
          <Col xs={24} lg={6}>
            <Space.Compact style={{ width: '100%' }}>
              <Select
                style={{ width: '65%' }}
                value={tenantSortBy}
                options={tenantSortOptions.map((opt) => ({ label: opt.label, value: opt.value }))}
                onChange={(value) => setTenantSortBy(value as 'task_count' | 'failure_rate')}
              />
              <Select
                style={{ width: '35%' }}
                value={tenantTopN}
                options={[10, 20, 50].map((n) => ({ label: `Top${n}`, value: n }))}
                onChange={(value) => setTenantTopN(value)}
              />
            </Space.Compact>
          </Col>
        </Row>
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card title="任务趋势" loading={loading}>
            <Line {...taskTrendConfig} />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="任务状态分布" loading={loading}>
            <Pie {...statusPieConfig} />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="成功/失败对比" loading={loading}>
            <Column {...successFailConfig} />
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card title="数据量趋势 (MB)" loading={loading}>
            <Line {...dataSizeConfig} />
          </Card>
        </Col>
        <Col span={24}>
          <Card title={tenantChartTitle} loading={loading}>
            <Column {...tenantTaskConfig} />
          </Card>
        </Col>
        <Col span={24}>
          <Card title={`租户统计明细（按${tenantSortBy === 'failure_rate' ? '失败率' : '任务数'}排序）`} loading={loading}>
            <Table
              rowKey="tenant_id"
              columns={tenantColumns}
              dataSource={tenantStats}
              pagination={false}
              scroll={{ x: 900 }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
}
