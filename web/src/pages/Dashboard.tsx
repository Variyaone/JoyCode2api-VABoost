// Modified by Variya, 2026-09-11: calendar-based usage report, not session telemetry.
import { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Collapse, Segmented, Skeleton, Space, Switch, Tabs } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useOutletContext } from 'react-router-dom';
import { api } from '../api';
import type { DashboardOutletContext } from '../layouts/MainLayout';
import { useRefreshableResource } from '../hooks/useRefreshableResource';
import type { Currency } from '../utils/costs';
import { usageReport } from '../utils/usageReport';
import type { UsageRange } from '../utils/usageReport';
import { DailyUsage, ModelUsage, UsageMetrics } from '../components/UsageReport';
import { usageEconomics } from '../utils/usageEconomics';
import ActivityHeatmap from '../components/ActivityHeatmap';
import ResourceStatus from '../components/ResourceStatus';
import OperationsDetails from './OperationsDetails';

export default function Dashboard() {
  const { autoRefresh, setAutoRefresh, health } = useOutletContext<DashboardOutletContext>();
  const [panel, setPanel] = useState('overview');
  const [range, setRange] = useState<UsageRange>('all');
  const [currency, setCurrency] = useState<Currency>(() => localStorage.getItem('jc_cost_currency') === 'USD' ? 'USD' : 'CNY');
  useEffect(() => { localStorage.setItem('jc_cost_currency', currency); }, [currency]);
  const costs = useRefreshableResource(api.getCosts, { autoRefresh });
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [detailsVisited, setDetailsVisited] = useState(false);
  const reportState = useMemo(() => {
    if (!costs.data) return { report: null, error: '' };
    try { return { report: usageReport(costs.data, range), error: '' }; }
    catch { return { report: null, error: '账本日期格式异常，无法安全计算日历统计。' }; }
  }, [costs.data, range]);
  const report = reportState.report;
  const economics = useMemo(() => report && costs.data ? usageEconomics(report, costs.data) : null, [report, costs.data]);
  const [activityRefresh, setActivityRefresh] = useState(0);
  const refresh = () => { setActivityRefresh(value => value + 1); return Promise.all([costs.refresh(), health.refresh()]); };
  return <div className="jc-page jc-usage-report">
    <header className="jc-report-title"><div className="jc-report-heading"><h1>用量与消耗</h1><p>用了多少、花了多少，以及消耗在哪些模型和时段。</p></div></header>
    <div className="jc-report-toolbar">
      <Tabs activeKey={panel} onChange={setPanel} items={[{ key: 'overview', label: 'Overview' }, { key: 'models', label: 'Models' }]} />
      <Segmented aria-label="统一统计时间范围" value={range} onChange={value => setRange(value as UsageRange)} options={[{ label: 'All', value: 'all' }, { label: '30d', value: '30d' }, { label: '7d', value: '7d' }]} />
    </div>
    <div className="jc-report-refresh"><ResourceStatus label="用量账本" resource={costs} />
      <Space wrap><Segmented aria-label="报告币种" value={currency} onChange={value => setCurrency(value as Currency)} options={[{ value: 'CNY', label: '¥ 人民币' }, { value: 'USD', label: '$ 美元' }]} /><label className="jc-auto-refresh"><Switch size="small" aria-label="30 秒自动刷新" checked={autoRefresh} onChange={setAutoRefresh} />自动刷新</label><Button aria-label="刷新用量报告" icon={<ReloadOutlined />} onClick={() => void refresh()} loading={costs.initialLoading || costs.refreshing || health.refreshing}>刷新</Button></Space>
    </div>
    {costs.initialLoading && <div className="jc-dot-loading" role="status" aria-label="正在读取用量"><span aria-hidden="true">•••</span><Skeleton paragraph={{ rows: 6 }} /></div>}
    {reportState.error && <Alert type="error" showIcon title={reportState.error} />}
    {report && economics && <>
      <p className="jc-report-scope">{report.start} 至 {report.end}，按服务器日历 {report.timezone}。All 指最早账本日期至今，不是终身用量或完整覆盖承诺；空白日期表示没有记录，不证明没有调用。{report.partial ? '部分日期早于已知覆盖期。' : ''}</p>
      {costs.data?.notice && <details className="jc-ledger-notice"><summary>数据口径与缺失说明{report.missing ? `（${report.missing} 次请求缺用量）` : ''}</summary><p>来源为代理用量账本；不是客户端会话记录。{costs.data.notice}</p></details>}
      <div role="tabpanel" aria-label={panel === 'overview' ? 'Overview' : 'Models'}>
        {panel === 'overview' ? <><UsageMetrics report={report} economics={economics} currency={currency} /><DailyUsage report={report} economics={economics} currency={currency} /><ModelUsage report={report} economics={economics} currency={currency} compact onMore={() => setPanel('models')} /><ActivityHeatmap key={`${report.start}/${report.end}`} report={report} autoRefresh={autoRefresh} refreshKey={activityRefresh} /></> : <ModelUsage report={report} economics={economics} currency={currency} />}
      </div>
    </>}
    <div className="jc-secondary-tools"><h2>其他数据与工具</h2><p>以下保留原始口径：今日运行统计、最近请求、累计费用、公开评测与候选能力实测，不受上方 All / 30d / 7d 控制。</p>
      <Collapse activeKey={detailsOpen ? ['details'] : []} onChange={keys => { const open = keys.includes('details'); setDetailsOpen(open); if (open) setDetailsVisited(true); }} items={[{ key: 'details', label: '运行明细、费用与模型能力', children: detailsVisited ? <OperationsDetails active={detailsOpen} costs={costs} currency={currency} onCurrencyChange={setCurrency} /> : null }]} />
    </div>
  </div>;
}
