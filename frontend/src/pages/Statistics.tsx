import { useEffect, useState } from 'react';
import { Card, Row, Col, DatePicker, Typography } from 'antd';
import { Line, Pie, Column } from '@ant-design/charts';
import dayjs from 'dayjs';
import api from '@/services/api';
import type { ApiResponse, DailyStatistics, TaskStatistics } from '@/types';

const { Title } = Typography;
const { RangePicker } = DatePicker;

export default function Statistics() {
  const [loading, setLoading] = useState(false);
  const [dailyStats, setDailyStats] = useState<DailyStatistics[]>([]);
  const [overviewStats, setOverviewStats] = useState<TaskStatistics | null>(null);
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs, dayjs.Dayjs]>([
    dayjs().subtract(30, 'day'),
    dayjs(),
  ]);

  useEffect(() => {
    fetchStatistics();
  }, [dateRange]);

  const fetchStatistics = async () => {
    setLoading(true);
    try {
      const [dailyRes, overviewRes] = await Promise.all([
        api.get<ApiResponse<DailyStatistics[]>>(
          `/admin/statistics/daily?start_date=${dateRange[0].format('YYYY-MM-DD')}&end_date=${dateRange[1].format('YYYY-MM-DD')}`
        ),
        api.get<ApiResponse<TaskStatistics>>('/admin/statistics/overview'),
      ]);

      if (dailyRes.data.code === 0) {
        setDailyStats(dailyRes.data.data);
      }
      if (overviewRes.data.code === 0) {
        setOverviewStats(overviewRes.data.data);
      }
    } catch (error) {
      console.error('Failed to fetch statistics:', error);
    } finally {
      setLoading(false);
    }
  };

  const taskTrendConfig = {
    data: dailyStats,
    xField: 'date',
    yField: 'task_count',
    seriesField: 'type',
    smooth: true,
    animation: {
      appear: {
        animation: 'path-in',
        duration: 1000,
      },
    },
  };

  const taskData = dailyStats.map((item) => [
    { date: item.date, type: '成功', count: item.success_count },
    { date: item.date, type: '失败', count: item.failed_count },
  ]).flat();

  const successFailConfig = {
    data: taskData,
    xField: 'date',
    yField: 'count',
    seriesField: 'type',
    isGroup: true,
    animation: {
      appear: {
        animation: 'scale-in',
        duration: 1000,
      },
    },
  };

  const statusPieData = overviewStats
    ? [
        { type: '待处理', value: overviewStats.pending_tasks },
        { type: '运行中', value: overviewStats.running_tasks },
        { type: '成功', value: overviewStats.success_tasks },
        { type: '失败', value: overviewStats.failed_tasks },
        { type: '已取消', value: overviewStats.canceled_tasks },
      ]
    : [];

  const statusPieConfig = {
    data: statusPieData,
    angleField: 'value',
    colorField: 'type',
    radius: 0.8,
    innerRadius: 0.6,
    label: {
      type: 'inner',
      offset: '-50%',
      content: '{value}',
      style: {
        textAlign: 'center',
        fontSize: 14,
      },
    },
    legend: {
      position: 'bottom' as const,
    },
    animation: {
      appear: {
        animation: 'grow-in',
        duration: 1000,
      },
    },
  };

  const dataSizeData = dailyStats.map((item) => ({
    date: item.date,
    size: Math.round(item.total_size / 1024 / 1024), // Convert to MB
  }));

  const dataSizeConfig = {
    data: dataSizeData,
    xField: 'date',
    yField: 'size',
    label: {
      position: 'middle' as const,
    },
    animation: {
      appear: {
        animation: 'scale-in',
        duration: 1000,
      },
    },
  };

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>
        统计报表
      </Title>

      <Card style={{ marginBottom: 16 }}>
        <RangePicker
          value={dateRange}
          onChange={(dates) => dates && setDateRange(dates as [dayjs.Dayjs, dayjs.Dayjs])}
        />
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
            <Column {...dataSizeConfig} />
          </Card>
        </Col>
      </Row>
    </div>
  );
}
