// Modified by Variya, 2026-09-11: independent light workspace navigation.
import { useEffect, useState } from 'react';
import { Button, Dropdown, Layout, Tag, Tooltip, Typography } from 'antd';
import { CheckCircleOutlined, LogoutOutlined, MenuOutlined, QuestionCircleOutlined, WarningOutlined } from '@ant-design/icons';
import { Link, Outlet, useLocation } from 'react-router-dom';
import ProjectCredits from '../components/ProjectCredits';
import BrandMark from '../components/BrandMark';
import RepoStarBadges from '../components/RepoStarBadges';
import useDocumentTitle from '../hooks/useDocumentTitle';
import { api, clearToken } from '../api';
import { brand } from '../brand';
import { useRefreshableResource } from '../hooks/useRefreshableResource';
import type { ResourceState } from '../components/ResourceStatus';

export interface DashboardOutletContext {
  autoRefresh: boolean;
  setAutoRefresh: (value: boolean) => void;
  health: ResourceState;
}
const navigation = [
  { path: '/dashboard', label: '工作台' },
  { path: '/accounts', label: '账号管理' },
  { path: '/settings', label: '设置' },
];

export default function MainLayout() {
  const location = useLocation();
  useDocumentTitle();
  const [autoRefresh, setAutoRefresh] = useState(() => localStorage.getItem('jc_auto_refresh') !== 'false');
  const health = useRefreshableResource(async signal => {
    const result = await api.getHealth(signal);
    if (result.status !== 'ok') throw new Error('代理状态异常');
    return result;
  }, { autoRefresh });
  useEffect(() => { localStorage.setItem('jc_auto_refresh', String(autoRefresh)); }, [autoRefresh]);
  const selected = location.pathname.startsWith('/accounts') ? '/accounts' : location.pathname.startsWith('/settings') ? '/settings' : '/dashboard';
  const items = navigation.map(item => ({ key: item.path, label: <Link to={item.path}>{item.label}</Link> }));

  return <Layout style={{ minHeight: '100vh' }}>
    <header className="jc-app-header">
      <Link to="/dashboard" className="jc-brand"><BrandMark /><span>{brand.name}</span></Link>
      <nav className="jc-top-nav" aria-label="主导航">
        {navigation.map(item => <Link key={item.path} to={item.path} aria-current={selected === item.path ? 'page' : undefined}>{item.label}</Link>)}
      </nav>
      <div className="jc-header-actions">
        <Dropdown menu={{ items, selectedKeys: [selected] }} trigger={['click']}><Button className="jc-mobile-nav" aria-label="打开导航" icon={<MenuOutlined />} /></Dropdown>
        <RepoStarBadges />
        <Tooltip title="退出登录"><Button type="text" aria-label="退出登录" icon={<LogoutOutlined />} onClick={() => {
          clearToken(); window.location.href = '/login';
        }} /></Tooltip>
      </div>
    </header>
    <div className="jc-connection-strip" role="region" aria-label="代理连接状态">
      <Tooltip title="仅表示上次检查时本地代理 HTTP 可达，不代表上游模型可用。">
        <Tag color={health.error ? 'error' : 'default'} icon={health.error ? <WarningOutlined /> : !health.data || health.stale ? <QuestionCircleOutlined /> : <CheckCircleOutlined />}>
          {health.error ? '状态获取失败' : !health.data ? '正在读取状态' : health.stale ? '代理状态待更新' : '代理可连接'}
        </Tag>
      </Tooltip>
      <Typography.Text type="secondary">{health.data ? `已配置 ${health.data.accounts} 个账号${health.error || health.stale ? '（上次记录）' : ''}` : '账号数量待读取'}</Typography.Text>
    </div>
    <main className="jc-app-main"><Outlet context={{ autoRefresh, setAutoRefresh, health } satisfies DashboardOutletContext} /></main>
    <ProjectCredits />
  </Layout>;
}
