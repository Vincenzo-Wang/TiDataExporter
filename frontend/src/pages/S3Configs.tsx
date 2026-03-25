import { useEffect, useState, useCallback } from 'react';
import { Table, Card, Tag, Button, Space, Modal, Form, Input, InputNumber, Select, message, Popconfirm } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import api from '@/services/api';
import type { ApiResponse, S3Config, PaginatedResponse } from '@/types';

export default function S3Configs() {
  const [loading, setLoading] = useState(false);
  const [configs, setConfigs] = useState<S3Config[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingConfig, setEditingConfig] = useState<S3Config | null>(null);
  const [form] = Form.useForm();

  const fetchConfigs = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.get<ApiResponse<PaginatedResponse<S3Config>>>(
        `/admin/s3-configs?page=${page}&page_size=${pageSize}`
      );
      if (response.data.code === 0) {
        setConfigs(response.data.data.items);
        setTotal(response.data.data.total);
      }
    } catch (error) {
      console.error('Failed to fetch S3 configs:', error);
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

  const handleEdit = (config: S3Config) => {
    setEditingConfig(config);
    form.setFieldsValue({
      name: config.name,
      provider: config.provider,
      endpoint: config.endpoint,
      bucket: config.bucket,
      region: config.region,
      path_prefix: config.path_prefix,
      status: config.status,
    });
    setModalVisible(true);
  };

  const handleDelete = async (id: number) => {
    try {
      const response = await api.delete<ApiResponse<unknown>>(`/admin/s3-configs/${id}`);
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
          `/admin/s3-configs/${editingConfig.id}`,
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
        const response = await api.post<ApiResponse<unknown>>('/admin/s3-configs', values);
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

  const columns: ColumnsType<S3Config> = [
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
      title: '厂商',
      dataIndex: 'provider',
      key: 'provider',
      width: 80,
      render: (provider: string) => (
        <Tag color={provider === 'aliyun' ? 'orange' : 'blue'}>
          {provider === 'aliyun' ? '阿里云' : 'AWS'}
        </Tag>
      ),
    },
    {
      title: 'Endpoint',
      dataIndex: 'endpoint',
      key: 'endpoint',
      width: 200,
      ellipsis: true,
    },
    {
      title: 'Bucket',
      dataIndex: 'bucket',
      key: 'bucket',
      width: 120,
    },
    {
      title: 'Region',
      dataIndex: 'region',
      key: 'region',
      width: 100,
    },
    {
      title: '路径前缀',
      dataIndex: 'path_prefix',
      key: 'path_prefix',
      width: 120,
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
        title="S3 配置管理"
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
        title={editingConfig ? '编辑 S3 配置' : '新建 S3 配置'}
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
            name="provider"
            label="存储厂商"
            rules={[{ required: true, message: '请选择存储厂商' }]}
            initialValue="aws"
          >
            <Select
              placeholder="请选择存储厂商"
              options={[
                { value: 'aws', label: 'AWS S3' },
                { value: 'aliyun', label: '阿里云 OSS' },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="endpoint"
            label="Endpoint"
            rules={[{ required: true, message: '请输入 Endpoint' }]}
          >
            <Input placeholder="https://s3.amazonaws.com 或 oss-cn-hangzhou.aliyuncs.com" />
          </Form.Item>
          <Form.Item
            name="bucket"
            label="Bucket"
            rules={[{ required: true, message: '请输入 Bucket 名称' }]}
          >
            <Input placeholder="请输入 Bucket 名称" />
          </Form.Item>
          <Form.Item name="region" label="Region" initialValue="us-east-1">
            <Input placeholder="请输入 Region (阿里云可不填)" />
          </Form.Item>
          <Form.Item
            name="access_key_id"
            label="Access Key ID"
            rules={editingConfig ? [] : [{ required: true, message: '请输入 Access Key ID' }]}
          >
            <Input placeholder="请输入 Access Key ID" />
          </Form.Item>
          <Form.Item
            name="secret_access_key"
            label="Secret Access Key"
            rules={editingConfig ? [] : [{ required: true, message: '请输入 Secret Access Key' }]}
          >
            <Input.Password placeholder="请输入 Secret Access Key" />
          </Form.Item>
          <Form.Item name="path_prefix" label="路径前缀">
            <Input placeholder="exports/" />
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
