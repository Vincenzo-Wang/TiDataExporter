import { useEffect, useState } from 'react';
import { Card, Descriptions, Tag, Progress, Button, Space, Modal, Typography, message } from 'antd';
import { ArrowLeftOutlined, StopOutlined, RedoOutlined, CopyOutlined } from '@ant-design/icons';
import { useParams, useNavigate } from 'react-router-dom';
import api from '@/services/api';
import type { ApiResponse, ExportTask, TaskStatus } from '@/types';

const { Text } = Typography;
const { Paragraph } = Typography;

const taskStatusMap: Record<TaskStatus, { color: string; text: string }> = {
  pending: { color: 'default', text: '待处理' },
  running: { color: 'processing', text: '运行中' },
  success: { color: 'success', text: '成功' },
  failed: { color: 'error', text: '失败' },
  canceled: { color: 'warning', text: '已取消' },
  expired: { color: 'default', text: '已过期' },
};

export default function TaskDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [task, setTask] = useState<ExportTask | null>(null);

  useEffect(() => {
    fetchTask();
  }, [id]);

  const fetchTask = async () => {
    if (!id) return;
    setLoading(true);
    try {
      const response = await api.get<ApiResponse<ExportTask>>(`/admin/tasks/${id}`);
      if (response.data.code === 0) {
        setTask(response.data.data);
      }
    } catch (error) {
      console.error('Failed to fetch task:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = async () => {
    if (!task) return;
    Modal.confirm({
      title: '确认取消',
      content: '确定要取消此任务吗？',
      onOk: async () => {
        try {
          const response = await api.post<ApiResponse<unknown>>(`/admin/tasks/${task.task_id}/cancel`);
          if (response.data.code === 0) {
            message.success('任务已取消');
            fetchTask();
          } else {
            message.error(response.data.message || '操作失败');
          }
        } catch {
          message.error('操作失败');
        }
      },
    });
  };

  const handleRetry = async () => {
    if (!task) return;
    try {
      const response = await api.post<ApiResponse<unknown>>(`/admin/tasks/${task.task_id}/retry`);
      if (response.data.code === 0) {
        message.success('任务已重新提交');
        fetchTask();
      } else {
        message.error(response.data.message || '操作失败');
      }
    } catch {
      message.error('操作失败');
    }
  };

  const copyFileUrl = () => {
    if (task?.file_url) {
      navigator.clipboard.writeText(task.file_url);
      message.success('已复制到剪贴板');
    }
  };

  const formatSize = (size: number) => {
    if (!size) return '-';
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`;
    if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(2)} MB`;
    return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
  };

  if (!task) {
    return <Card loading={loading} />;
  }

  const statusInfo = taskStatusMap[task.status] || { color: 'default', text: task.status };

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/tasks')}>
          返回列表
        </Button>
        {(task.status === 'pending' || task.status === 'running') && (
          <Button danger icon={<StopOutlined />} onClick={handleCancel}>
            取消任务
          </Button>
        )}
        {task.status === 'failed' && (
          <Button icon={<RedoOutlined />} onClick={handleRetry}>
            重试任务
          </Button>
        )}
      </Space>

      <Card title={`任务详情 - #${task.task_id}`} loading={loading}>
        <Descriptions column={2} bordered>
          <Descriptions.Item label="任务ID">{task.task_id}</Descriptions.Item>
          <Descriptions.Item label="任务名称">{task.task_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="租户">{task.tenant_name || `ID: ${task.tenant_id}`}</Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag color={statusInfo.color}>{statusInfo.text}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label="进度">
            <Progress percent={task.progress || 0} />
          </Descriptions.Item>
          <Descriptions.Item label="优先级">{task.priority}</Descriptions.Item>
          <Descriptions.Item label="TiDB 配置">{task.tidb_config_name || `ID: ${task.tidb_config_id}`}</Descriptions.Item>
          <Descriptions.Item label="S3 配置">{task.s3_config_name || `ID: ${task.s3_config_id}`}</Descriptions.Item>
          <Descriptions.Item label="文件类型">{task.filetype?.toUpperCase() || '-'}</Descriptions.Item>
          <Descriptions.Item label="压缩方式">{task.compress?.toUpperCase() || '无'}</Descriptions.Item>
          <Descriptions.Item label="文件大小">{formatSize(task.file_size)}</Descriptions.Item>
          <Descriptions.Item label="数据行数">{task.row_count?.toLocaleString() || '-'}</Descriptions.Item>
          <Descriptions.Item label="重试次数">{task.retry_count} / {task.max_retries || 3}</Descriptions.Item>
          <Descriptions.Item label="保留时间">{task.retention_hours} 小时</Descriptions.Item>
          <Descriptions.Item label="创建时间">
            {task.created_at ? new Date(task.created_at).toLocaleString('zh-CN') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="开始时间">
            {task.started_at ? new Date(task.started_at).toLocaleString('zh-CN') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="完成时间">
            {task.completed_at ? new Date(task.completed_at).toLocaleString('zh-CN') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="过期时间">
            {task.expires_at ? new Date(task.expires_at).toLocaleString('zh-CN') : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="文件地址" span={2}>
            <Space>
              <Text copyable={{ text: task.file_url }}>{task.file_url || '-'}</Text>
              {task.file_url && (
                <Button type="link" size="small" icon={<CopyOutlined />} onClick={copyFileUrl}>
                  复制
                </Button>
              )}
            </Space>
          </Descriptions.Item>
          <Descriptions.Item label="SQL 语句" span={2}>
            <Paragraph
              style={{ marginBottom: 0, maxWidth: 600 }}
              ellipsis={{ rows: 4, expandable: true, symbol: '展开' }}
            >
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{task.sql_text}</pre>
            </Paragraph>
          </Descriptions.Item>
          {task.error_message && (
            <Descriptions.Item label="错误信息" span={2}>
              <Text type="danger">{task.error_message}</Text>
            </Descriptions.Item>
          )}
          {task.cancel_reason && (
            <Descriptions.Item label="取消原因" span={2}>
              <Text type="warning">{task.cancel_reason}</Text>
            </Descriptions.Item>
          )}
        </Descriptions>
      </Card>
    </div>
  );
}
