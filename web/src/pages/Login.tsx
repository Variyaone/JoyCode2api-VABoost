// Modified by Variya 2026-09-11: restrained light UI and maintainer identity.
import React, { useState } from 'react';
import { Form, Input, Button, message, Typography, Tooltip } from 'antd';
import { LockOutlined, UserOutlined, QuestionCircleOutlined, GithubOutlined } from '@ant-design/icons';
import { useNavigate, Link } from 'react-router-dom';
import BrandMark from '../components/BrandMark';
import ProjectCredits from '../components/ProjectCredits';
import { authApi, setToken } from '../api';

const { Title, Text } = Typography;

const LoginPage: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (values: { password: string }) => {
    setLoading(true);
    try {
      const result = await authApi.login(values.password);
      setToken(result.token);
      message.success('登录成功');
      navigate('/dashboard');
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '登录失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="jc-auth-bg">
      <Tooltip title="joyCode2api-VABoost 源码仓库（GitHub / Gitee 同步）">
        <a
          href="https://github.com/variyaone/JoyCode2api-VABoost"
          target="_blank"
          rel="noopener noreferrer"
          style={{
            position: 'absolute',
            top: 20,
            right: 24,
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            color: '#5966A6',
            fontSize: 13,
            textDecoration: 'none',
          }}
        >
          <GithubOutlined style={{ fontSize: 18 }} />
          VABoost
        </a>
      </Tooltip>
      <div className="jc-auth-card">
        <div className="jc-auth-logo">
          <BrandMark size={40} />
        </div>
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <Title level={3} style={{ marginBottom: 4 }}>JoyCode 代理</Title>
          <Text type="secondary">请输入 root 密码登录</Text>
        </div>
        <Form onFinish={handleSubmit} size="large">
          <Form.Item name="username" initialValue="root">
            <Input prefix={<UserOutlined />} disabled />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password prefix={<LockOutlined />} placeholder="root 密码" autoFocus />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Button
              type="primary"
              htmlType="submit"
              loading={loading}
              block
              style={{ height: 44 }}
            >
              登录
            </Button>
          </Form.Item>
        </Form>
        <div style={{ textAlign: 'center', marginTop: 16 }}>
          <Link
            to="/forgot-password"
            style={{ color: '#646B78', fontSize: 13, display: 'inline-flex', alignItems: 'center', gap: 4 }}
          >
            <QuestionCircleOutlined />
            忘记密码？
          </Link>
        </div>
        <ProjectCredits />
      </div>
    </div>
  );
};

export default LoginPage;
