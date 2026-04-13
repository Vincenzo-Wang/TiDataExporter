import { useEffect, useState, useCallback } from 'react';
import { Table, Card, Tag, Button, Space, Modal, Form, Input, InputNumber, Select, message, Popconfirm } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import api from '@/services/api';
import type { ApiResponse, TiDBConfig, PaginatedResponse } from '@/types';

export default function TiDBConfigs() {
  const [loading, setLoading] = useState(false);
  const [configs, setConfigs] = useState<TiDBConfig[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingConfig, setEditingConfig] = useState<TiDBConfig | null>(null);
  const [form] = Form.useForm();

  const fetchConfigs = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.get<ApiResponse<PaginatedResponse<TiDBConfig>>>(
        `/admin/tidb-configs?page=${page}&page_size=${pageSize}`
      );
      if (response.data.code === 0) {
        setConfigs(response.data.data.items);
        setTotal(response.data.data.total);
      }
    } catch (error) {
      console.error('Failed to fetch TiDB configs:', error);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => {
    fetchConfigs();
  }, [fetchConfigs]);

  const handleCreate = () => {
    setEditingConfig(null);
    form.resetFields();
    setModalVisible(true);
  };

  const handleEdit = (config: TiDBConfig) => {
    setEditingConfig(config);
    form.setFieldsValue({
      tenant_id: config.tenant_id,
      name: config.name,
      host: config.host,
      port: config.port,
      username: config.username,
      database: config.database,
      ssl_mode: config.ssl_mode,
      status: config.status,
    });
    setModalVisible(true);
  };

  const handleDelete = async (id: number) => {
    try {
      const response = await api.delete<ApiResponse<unknown>>(`/admin/tidb-configs/${id}`);
      if (response.data.code === 0) {
        message.success('删除成功');
        fetchConfigs();
      } else {
        message.error(response.data.message);
      }
    } catch {
      message.error('删除失败');
    }
  };

  const handleSubmit = async (values: Record<string, unknown>) => {
    try {
      if (editingConfig) {
        const response = await api.put<ApiResponse<unknown>>(
          `/admin/tidb-configs/${editingConfig.id}`,
          values
        );
        if (response.data.code === 0) {
          message.success('更新成功');
          setModalVisible(false);
          fetchConfigs();
        } else {
          message.error(response.data.message);
        }
      } else {
        const response = await api.post<ApiResponse<unknown>>('/admin/tidb-configs', values);
        if (response.data.code === 0) {
          message.success('创建成功');
          setModalVisible(false);
          fetchConfigs();
        } else {
          message.error(response.data.message);
        }
      }
    } catch {
      message.error('操作失败');
    }
  };

  const columns: ColumnsType<TiDBConfig> = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 150,
    },
    {
      title: '租户ID',
      dataIndex: 'tenant_id',
      key: 'tenant_id',
      width: 80,
    },
    {
      title: '主机',
      dataIndex: 'host',
      key: 'host',
      width: 150,
    },
    {
      title: '端口',
      dataIndex: 'port',
      key: 'port',
      width: 80,
    },
    {
      title: '用户名',
      dataIndex: 'username',
      key: 'username',
      width: 100,
    },
    {
      title: '数据库',
      dataIndex: 'database',
      key: 'database',
      width: 120,
    },
    {
      title: 'SSL 模式',
      dataIndex: 'ssl_mode',
      key: 'ssl_mode',
      width: 100,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (status: number) => (
        <Tag color={status === 1 ? 'success' : 'error'}>{status === 1 ? '启用' : '禁用'}</Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (time: string) => new Date(time).toLocaleString('zh-CN'),
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_, record) => (
        <Space size="small">
          <Button type="link" size="small" onClick={() => handleEdit(record)}>
            编辑
          </Button>
          <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Card
        title="TiDB 配置管理"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            新建配置
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={configs}
          rowKey="id"
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

      <Modal
        title={editingConfig ? '编辑 TiDB 配置' : '新建 TiDB 配置'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="tenant_id"
            label="租户ID"
            rules={[{ required: true, message: '请输入租户ID' }]}
          >
            <InputNumber min={1} style={{ width: '100%' }} disabled={!!editingConfig} />
          </Form.Item>
          <Form.Item
            name="name"
            label="配置名称"
            rules={[{ required: true, message: '请输入配置名称' }]}
          >
            <Input placeholder="请输入配置名称" />
          </Form.Item>
          <Form.Item
            name="host"
            label="主机地址"
            rules={[{ required: true, message: '请输入主机地址' }]}
          >
            <Input placeholder="请输入主机地址" />
          </Form.Item>
          <Form.Item name="port" label="端口" initialValue={4000}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input placeholder="请输入用户名" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={editingConfig ? [] : [{ required: true, message: '请输入密码' }]}
          >
            <Input.Password placeholder="请输入密码" />
          </Form.Item>
          <Form.Item
            name="database"
            label="数据库"
            rules={[{ required: true, message: '请输入数据库名' }]}
          >
            <Input placeholder="请输入数据库名" />
          </Form.Item>
          <Form.Item name="ssl_mode" label="SSL 模式" initialValue="disabled">
            <Select
              options={[
                { value: 'disabled', label: '禁用' },
                { value: 'preferred', label: '优先' },
                { value: 'required', label: '必需' },
                { value: 'verify_ca', label: '验证 CA' },
                { value: 'verify_identity', label: '验证身份' },
              ]}
            />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select
              options={[
                { value: 1, label: '启用' },
                { value: 0, label: '禁用' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
