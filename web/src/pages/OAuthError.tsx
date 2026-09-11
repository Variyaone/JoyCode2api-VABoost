// Modified by Variya 2026-09-11: restrained light UI and shared branding.
import React from 'react';
import { Button, Typography } from 'antd';
import { CloseCircleOutlined, LoginOutlined, HomeOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import BrandMark from '../components/BrandMark';
import ProjectCredits from '../components/ProjectCredits';
import { colors } from '../theme';

const { Title, Text, Paragraph } = Typography;

const OAuthError: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const error = searchParams.get('error') || '未知错误';

  return (
    <div className="jc-auth-bg">
      <div className="jc-auth-card" style={{ width: 500, textAlign: 'center' }}>
        <div className="jc-auth-logo">
          <BrandMark size={40} />
        </div>
        <Title level={3}><CloseCircleOutlined style={{ color: colors.danger, fontSize: 20, marginRight: 8 }} />OAuth 授权失败</Title>
        <Paragraph type="secondary" style={{ fontSize: 14 }}>
          授权过程中发生错误，账号未能添加成功。
        </Paragraph>
        <div style={{
          background: '#F5F6F8',
          border: '1px solid #E1E4EA',
          borderRadius: 8,
          padding: '12px 16px',
          marginBottom: 24,
          textAlign: 'left',
        }}>
          <Text type="danger" style={{ fontSize: 13, wordBreak: 'break-all' }}>
            {error}
          </Text>
        </div>
        <div style={{ display: 'flex', gap: 12, justifyContent: 'center', flexWrap: 'wrap' }}>
          <Button
            icon={<LoginOutlined />}
            onClick={() => navigate('/accounts')}
          >
            返回账号管理
          </Button>
          <Button
            type="primary"
            icon={<HomeOutlined />}
            onClick={() => navigate('/dashboard')}
          >
            返回首页
          </Button>
        </div>
        <ProjectCredits />
      </div>
    </div>
  );
};

export default OAuthError;
