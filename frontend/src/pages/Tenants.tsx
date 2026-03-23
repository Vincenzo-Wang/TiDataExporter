import { useEffect, useState, useCallback } from 'react';
import { Table, Card, Tag, Button, Space, Modal, Form, Input, InputNumber, Select, message, Popconfirm } from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined, KeyOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import api from '@/services/api';
import type { ApiResponse, Tenant, PaginatedResponse } from '@/types';

export default function Tenants() {
  const [loading, setLoading] = useState(false);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingTenant, setEditingTenant] = useState<Tenant | null>(null);
  const [form] = Form.useForm();

  const fetchTenants = useCallback(async () => {
    setLoading(true);
    try {
      const response = await api.get<ApiResponse<PaginatedResponse<Tenant>>>(
        `/admin/tenants?page=${page}&page_size=${pageSize}`
      );
      if (response.data.code === 0) {
        setTenants(response.data.data.items);
        setTotal(response.data.data.total);
      }
    } catch (error) {
      console.error('Failed to fetch tenants:', error);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => {
    fetchTenants();
  }, [fetchTenants]);

  const handleCreate = () => {
    setEditingTenant(null);
    form.resetFields();
    setModalVisible(true);
  };

  const handleEdit = (tenant: Tenant) => {
    setEditingTenant(tenant);
    form.setFieldsValue({
      name: tenant.name,
      code: tenant.code,
      contact_email: tenant.contact_email,
      quota_daily: tenant.quota_daily,
      quota_monthly: tenant.quota_monthly,
      status: tenant.status,
    });
    setModalVisible(true);
  };

  const handleDelete = async (id: number) => {
    try {
      const response = await api.delete<ApiResponse<unknown>>(`/admin/tenants/${id}`);
      if (response.data.code === 0) {
        message.success('删除成功');
        fetchTenants();
      } else {
        message.error(response.data.message);
      }
    } catch {
      message.error('删除失败');
    }
  };

  const handleRegenerateKeys = async (id: number) => {
    try {
      const response = await api.post<ApiResponse<{ api_key: string; api_secret: string }>>(
        `/admin/tenants/${id}/regenerate-keys`
      );
      if (response.data.code === 0) {
        Modal.success({
          title: '密钥已重新生成',
          content: (
            <div>
              <p>API Key: {response.data.data.api_key}</p>
              <p>API Secret: {response.data.data.api_secret}</p>
              <p style={{ color: '#ff4d4f' }}>请妥善保存，Secret 只会显示一次！</p>
            </div>
          ),
        });
        fetchTenants();
      } else {
        message.error(response.data.message);
      }
    } catch {
      message.error('操作失败');
    }
  };

  const handleSubmit = async (values: Record<string, unknown>) => {
    try {
      if (editingTenant) {
        const response = await api.put<ApiResponse<unknown>>(
          `/admin/tenants/${editingTenant.id}`,
          values
        );
        if (response.data.code === 0) {
          message.success('更新成功');
          setModalVisible(false);
          fetchTenants();
        } else {
          message.error(response.data.message);
        }
      } else {
        const response = await api.post<ApiResponse<{ api_key: string; api_secret: string }>>(
          '/admin/tenants',
          values
        );
        if (response.data.code === 0) {
          Modal.success({
            title: '租户创建成功',
            content: (
              <div>
                <p>API Key: {response.data.data.api_key}</p>
                <p>API Secret: {response.data.data.api_secret}</p>
                <p style={{ color: '#ff4d4f' }}>请妥善保存，Secret 只会显示一次！</p>
              </div>
            ),
          });
          setModalVisible(false);
          fetchTenants();
        } else {
          message.error(response.data.message);
        }
      }
    } catch {
      message.error('操作失败');
    }
  };

  const columns: ColumnsType<Tenant> = [
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
      title: '编码',
      dataIndex: 'code',
      key: 'code',
      width: 100,
    },
    {
      title: '联系邮箱',
      dataIndex: 'contact_email',
      key: 'contact_email',
      width: 200,
    },
    {
      title: 'API Key',
      dataIndex: 'api_key',
      key: 'api_key',
      width: 200,
      ellipsis: true,
    },
    {
      title: '日配额',
      key: 'daily_quota',
      width: 120,
      render: (_, record) => `${record.quota_used_today}/${record.quota_daily}`,
    },
    {
      title: '月配额',
      key: 'monthly_quota',
      width: 120,
      render: (_, record) => `${record.quota_used_month}/${record.quota_monthly}`,
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
      width: 180,
      render: (_, record) => (
        <Space size="small">
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => handleEdit(record)}>
            编辑
          </Button>
          <Button type="link" size="small" icon={<KeyOutlined />} onClick={() => handleRegenerateKeys(record.id)}>
            重置密钥
          </Button>
          <Popconfirm
            title="确定删除此租户？"
            onConfirm={() => handleDelete(record.id)}
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
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
        title="租户管理"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            新建租户
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={tenants}
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
        title={editingTenant ? '编辑租户' : '新建租户'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={() => form.submit()}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="name"
            label="租户名称"
            rules={[{ required: true, message: '请输入租户名称' }]}
          >
            <Input placeholder="请输入租户名称" />
          </Form.Item>
          <Form.Item
            name="code"
            label="租户编码"
            rules={[{ required: true, message: '请输入租户编码' }]}
          >
            <Input placeholder="请输入租户编码" disabled={!!editingTenant} />
          </Form.Item>
          <Form.Item
            name="contact_email"
            label="联系邮箱"
            rules={[
              { required: true, message: '请输入联系邮箱' },
              { type: 'email', message: '邮箱格式不正确' },
            ]}
          >
            <Input placeholder="请输入联系邮箱" />
          </Form.Item>
          <Form.Item name="quota_daily" label="日配额" initialValue={100}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="quota_monthly" label="月配额" initialValue={3000}>
            <InputNumber min={0} style={{ width: '100%' }} />
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
