// Modified by Variya, 2026-09-11: light workspace information architecture.
import { useState } from 'react';
import { Alert, Button, Card, Collapse, Empty, Skeleton, Space, Switch, Table, Tabs, Tag, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { Link, useOutletContext } from 'react-router-dom';
import { api, accountDisplayName } from '../api';
import type { Account } from '../api';
import type { DashboardOutletContext } from '../layouts/MainLayout';
import { useRefreshableResource } from '../hooks/useRefreshableResource';
import AccountCredentialStatus, { credentialState } from '../components/AccountCredentialStatus';
import CostOverview from '../components/CostOverview';
import ModelBenchmarks from '../components/ModelBenchmarks';
import CandidateAudit from '../components/CandidateAudit';
import ResourceStatus from '../components/ResourceStatus';
import { CapabilityRecords, MoreStats, RecentRequests } from '../components/DashboardDetails';
import DashboardTrends from '../components/DashboardTrends';
import type { Currency } from '../utils/costs';
import type { CostSnapshot } from '../api';
import type { ResourceState } from '../components/ResourceStatus';

type Panel = 'live' | 'costs' | 'models';

export default function OperationsDetails({ costs, currency, onCurrencyChange, active = true }: {
  costs: ResourceState & { data: CostSnapshot | null; initialLoading: boolean };
  currency: Currency; onCurrencyChange: (value: Currency) => void; active?: boolean;
}) {
  const { autoRefresh, setAutoRefresh, health } = useOutletContext<DashboardOutletContext>();
  const [panel, setPanel] = useState<Panel>('live');

  const live = { enabled: active && panel === 'live', autoRefresh };
  const stats = useRefreshableResource(api.getStats, live);
  const accounts = useRefreshableResource(api.listAccounts, live);
  const logs = useRefreshableResource(signal => api.getRecentLogs(50, signal), live);
  const benchmarks = useRefreshableResource(api.getModelBenchmarks, { enabled: active && panel === 'models', autoRefresh: false });
  const capabilities = useRefreshableResource(api.getModelCapabilities, { enabled: active && panel === 'models', autoRefresh: false });

  const currentResources = panel === 'live' ? [stats, accounts, logs, costs] : panel === 'costs' ? [costs] : [benchmarks, capabilities];
  const refreshing = health.refreshing || currentResources.some(r => r.refreshing || r.initialLoading);
  const refresh = () => Promise.all([health.refresh(), ...currentResources.map(r => r.refresh())]);
  const data = stats.data;
  const accountRows = accounts.data ?? [];
  const counts = accountRows.reduce((n, a) => { n[credentialState(a.credential_valid)]++; return n; }, { passed: 0, failed: 0, unknown: 0 });

  const livePanel = <div className="jc-panel-stack">
    <ResourceStatus label="运行统计" resource={stats} />
    {stats.initialLoading && <Card><Skeleton active paragraph={{ rows: 4 }} /></Card>}
    <div className="jc-operations-grid">
      <div>{data && <DashboardTrends stats={data} at={stats.lastSuccessAt ?? Date.now()} />}</div>
      <aside className="jc-runtime-aside" aria-label="运行摘要">
        <Typography.Title level={4}>运行摘要</Typography.Title>
        <ResourceStatus label="代理连接" resource={health} />
        <p>代理可连接不代表上游模型实时可用。</p>
        <Typography.Title level={5}>账号历史校验</Typography.Title>
        <p>{accounts.data ? `通过 ${counts.passed}，失败 ${counts.failed}，未知 ${counts.unknown}` : '尚未读取'}</p>
        <p>校验失败可能是凭据或网络问题，不直接等于过期。</p>
        <Link to="/accounts">管理账号</Link>
        <ResourceStatus label="账号摘要" resource={accounts} />
      </aside>
    </div>
    <section aria-label="最近请求">
      <ResourceStatus label="请求日志" resource={logs} />
      {logs.initialLoading ? <Card><Skeleton active paragraph={{ rows: 3 }} /></Card> : <RecentRequests recentLogs={logs.data?.logs ?? []} />}
    </section>
    <Card size="small" title="账号历史校验" extra={<Link to="/accounts">管理账号</Link>}>
      <ResourceStatus label="账号" resource={accounts} />
      <div className="jc-account-summary">
        <Typography.Text>{accounts.data ? `通过 ${counts.passed} · 失败 ${counts.failed} · 未知 ${counts.unknown}` : '校验数量待读取'}</Typography.Text>
        <Typography.Text type="secondary">后台历史记录，非实时上游探测；校验失败不一定是凭据过期。</Typography.Text>
      </div>
      <Table<Account> rowKey="user_id" size="small" dataSource={accountRows} loading={accounts.initialLoading} pagination={false} scroll={{ x: 660 }}
        locale={{ emptyText: accounts.error ? '账号信息暂不可用，请重试' : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未配置账号"><Link to="/accounts">添加账号</Link></Empty> }}
        columns={[
          { title: '账号', render: (_, a) => <Typography.Text strong>{accountDisplayName(a)}</Typography.Text> },
          { title: '默认模型', dataIndex: 'default_model', render: (m: string) => m ? <Tag>{m}</Tag> : '—' },
          { title: '今日请求', align: 'right', render: (_, a) => a.today_requests.toLocaleString() },
          { title: '校验记录', width: 260, render: (_, a) => <AccountCredentialStatus account={a} /> },
        ]} />
    </Card>
    {data && <Collapse items={[{ key: 'more', label: '更多统计：累计用量、响应质量、模型与账号分布', children: <MoreStats stats={data} /> }]} />}
    {data && data.total_requests === 0 && (data.all_time?.total_requests ?? 0) === 0 &&
      <Alert type="info" showIcon title="暂无请求数据" description="配置账号并通过本地代理发起请求后，即可查看运行统计。" />}
  </div>;

  return <div className="jc-operations-details">
    <div className="jc-dashboard-toolbar">
      <div><Typography.Title level={2}>{panel === 'live' ? '运行明细' : panel === 'costs' ? '费用明细' : '模型参考'}</Typography.Title></div>
      <Space wrap className="jc-refresh-controls">
        <label className="jc-auto-refresh"><Switch size="small" checked={autoRefresh} onChange={setAutoRefresh} aria-label="30 秒自动刷新" />30 秒自动刷新</label>
        <Button icon={<ReloadOutlined />} onClick={() => void refresh()} loading={refreshing}>刷新当前页</Button>
      </Space>
    </div>
    <div className="jc-refresh-hint"><ResourceStatus label="代理连接" resource={health} />
      <Typography.Text type="secondary">{!autoRefresh ? '自动刷新已关闭，可手动刷新。' : panel === 'models' ? '模型参考仅手动重读；代理连接继续定时检查。' : '后台暂停，回到前台时更新过期数据。'}</Typography.Text>
    </div>
    <Tabs activeKey={panel} onChange={key => setPanel(key as Panel)} destroyOnHidden={false} items={[
      { key: 'live', label: '运行概览', children: livePanel },
      { key: 'costs', label: '费用明细', children: <CostOverview resource={costs} currency={currency} onCurrencyChange={onCurrencyChange} /> },
      { key: 'models', label: '模型参考', children: <div className="jc-panel-stack">
        <CandidateAudit />
        <ModelBenchmarks resource={benchmarks} />
        <ResourceStatus label="历史能力记录" resource={capabilities} snapshot />
        {capabilities.initialLoading ? <Card><Skeleton active /></Card> : <CapabilityRecords caps={capabilities.data?.models ?? []} />}
      </div> },
    ]} />
  </div>;
}
