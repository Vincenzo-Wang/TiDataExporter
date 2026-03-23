import { useEffect, useState } from 'react';
import { Card, Descriptions, Button, Space, Tag } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { useParams, useNavigate } from 'react-router-dom';
import api from '@/services/api';
import type { ApiResponse, Tenant } from '@/types';

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [tenant, setTenant] = useState<Tenant | null>(null);

  useEffect(() => {
    fetchTenant();
  }, [id]);

  const fetchTenant = async () => {
    if (!id) return;
    setLoading(true);
    try {
      const response = await api.get<ApiResponse<Tenant>>(`/admin/tenants/${id}`);
      if (response.data.code === 0) {
        setTenant(response.data.data);
      }
    } catch (error) {
      console.error('Failed to fetch tenant:', error);
    } finally {
      setLoading(false);
    }
  };

  if (!tenant) {
    return <Card loading={loading} />;
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/tenants')}>
          返回列表
        </Button>
      </Space>

      <Card title={`租户详情 - ${tenant.name}`} loading={loading}>
        <Descriptions column={2} bordered>
          <Descriptions.Item label="ID">{tenant.id}</Descriptions.Item>
          <Descriptions.Item label="名称">{tenant.name}</Descriptions.Item>
          <Descriptions.Item label="编码">{tenant.code}</Descriptions.Item>
          <Descriptions.Item label="联系邮箱">{tenant.contact_email}</Descriptions.Item>
          <Descriptions.Item label="API Key">{tenant.api_key}</Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag color={tenant.status === 1 ? 'success' : 'error'}>
              {tenant.status === 1 ? '启用' : '禁用'}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="日配额">{tenant.quota_daily}</Descriptions.Item>
          <Descriptions.Item label="日已用">{tenant.quota_used_today}</Descriptions.Item>
          <Descriptions.Item label="月配额">{tenant.quota_monthly}</Descriptions.Item>
          <Descriptions.Item label="月已用">{tenant.quota_used_month}</Descriptions.Item>
          <Descriptions.Item label="创建时间">
            {new Date(tenant.created_at).toLocaleString('zh-CN')}
          </Descriptions.Item>
          <Descriptions.Item label="更新时间">
            {new Date(tenant.updated_at).toLocaleString('zh-CN')}
          </Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}
