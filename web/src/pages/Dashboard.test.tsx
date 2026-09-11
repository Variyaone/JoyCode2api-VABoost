import { ConfigProvider } from 'antd';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { useEffect, useState } from 'react';
import { MemoryRouter, Outlet, Route, Routes, useOutletContext } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api';
import type { Account, BenchmarkSnapshot, CostSnapshot, RequestLog, Stats } from '../api';
import { useRefreshableResource } from '../hooks/useRefreshableResource';
import MainLayout from '../layouts/MainLayout';
import type { DashboardOutletContext } from '../layouts/MainLayout';
import ResourceStatus from '../components/ResourceStatus';
import type { Currency } from '../utils/costs';
import OperationsDetails from './OperationsDetails';

// Keep the actual tab, table, resource-status and hook implementations. Charts
// alone need layout/SVG facilities that jsdom does not provide.
vi.mock('recharts', () => {
  const Chart = () => null;
  return {
    ResponsiveContainer: Chart, Area: Chart, AreaChart: Chart, Bar: Chart,
    BarChart: Chart, CartesianGrid: Chart, ComposedChart: Chart, Legend: Chart,
    Line: Chart, Tooltip: Chart, XAxis: Chart, YAxis: Chart,
  };
});

const START = new Date('2026-09-11T00:00:00.000Z');
const stats: Stats = {
  total_requests: 125, total_input_tokens: 1500, total_output_tokens: 500,
  accounts_count: 1, avg_latency_ms: 2300, error_count: 5, stream_count: 100,
  success_count: 120, by_model: [], by_account: [], hourly: [],
  all_time: { total_requests: 500, total_input_tokens: 6000, total_output_tokens: 2000, error_count: 20 },
};
const accounts: Account[] = [{
  user_id: 'test-account', nickname: 'Retained account', remark: '', api_token: '',
  is_default: true, default_model: 'Test-model', display_order: 0, active_sessions: 0,
  total_requests: 125, today_requests: 125, total_tokens: 2000, today_tokens: 2000,
  credential_valid: 1,
}];
const costs: CostSnapshot = {
  currency: 'USD', price_version: 'test-price', collected_at: '2026-09-10',
  today: '2026-09-11', timezone: 'Asia/Shanghai', coverage_start: '2026-09-10',
  notice: 'Test cost ledger notice', rates: [],
  rows: [
    { day: '2026-09-11', model: 'Cost-model', price_version: 'test-price', requests: 5,
      input_tokens: 100, output_tokens: 50, missing_usage: 0, input_rate: 1, output_rate: 2,
      amount_tenth_micro_usd: 10_000_000 },
    { day: '2026-09-10', model: 'Cost-model', price_version: 'test-price', requests: 2,
      input_tokens: 100, output_tokens: 50, missing_usage: 0, input_rate: 1, output_rate: 2,
      amount_tenth_micro_usd: 20_000_000 },
  ],
};
const benchmarks: BenchmarkSnapshot = {
  schema_version: 1, collected_at: '2026-09-09', notice: 'Test benchmark snapshot notice', sources: [],
  models: [
    { id: 'Atlas-high', score: 90 }, { id: 'Atlas-low', score: 20 }, { id: 'Other-model', score: 60 },
  ].map(({ id, score }) => ({
    id, mapping: 'name_match', note: 'Verified test fixture',
    results: [{ source_id: 'aa', public_model: id, variant: 'standard', score,
      url: 'https://example.test/benchmark', published_at: '2026-09-01', footnote: '' }],
  })),
};
const logs: RequestLog[] = Array.from({ length: 15 }, (_, index) => ({
  id: index + 1, user_id: 'test-account', model: `Request-model-${index + 1}`,
  endpoint: '/v1/test', stream: false, status_code: 200, latency_ms: 100,
  error_message: '', input_tokens: 100, output_tokens: 10, created_at: '2026-09-11 08:00:00',
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function TestOutlet({ initiallyAutomatic = true }: { initiallyAutomatic?: boolean }) {
  const [autoRefresh, setAutoRefresh] = useState(initiallyAutomatic);
  const health = useRefreshableResource(api.getHealth, { autoRefresh });
  return <Outlet context={{ autoRefresh, setAutoRefresh, health } satisfies DashboardOutletContext} />;
}

// Mirror the Usage report's ownership contract without rendering/testing its UI:
// costs poll independently of the inner operations tab; currency lives above it.
// The output is a test-only probe of the owner, not a replacement overview KPI.
function TestCostOwner() {
  const { autoRefresh } = useOutletContext<DashboardOutletContext>();
  const costResource = useRefreshableResource(api.getCosts, { autoRefresh });
  const [currency, setCurrency] = useState<Currency>(() => localStorage.getItem('jc_cost_currency') === 'USD' ? 'USD' : 'CNY');
  useEffect(() => { localStorage.setItem('jc_cost_currency', currency); }, [currency]);
  return <>
    <ResourceStatus label="用量账本" resource={costResource} />
    <output data-testid="cost-owner">{JSON.stringify({ currency, data: costResource.data })}</output>
    <OperationsDetails costs={costResource} currency={currency} onCurrencyChange={setCurrency} />
  </>;
}

function ownerCosts(): { currency: Currency; data: CostSnapshot | null } {
  return JSON.parse(screen.getByTestId('cost-owner').textContent!);
}

function renderOperations(options: { autoRefresh?: boolean; realLayout?: boolean } = {}) {
  return render(<ConfigProvider theme={{ token: { motion: false } }}>
    <MemoryRouter initialEntries={['/dashboard']}>
      <Routes>
        <Route element={options.realLayout ? <MainLayout /> : <TestOutlet initiallyAutomatic={options.autoRefresh ?? true} />}>
          <Route path="/dashboard" element={<TestCostOwner />} />
        </Route>
      </Routes>
    </MemoryRouter>
  </ConfigProvider>);
}

async function flush() {
  await act(async () => { await Promise.resolve(); });
}

async function advance(ms: number) {
  await act(async () => { await vi.advanceTimersByTimeAsync(ms); });
}

async function tab(name: string) {
  fireEvent.click(screen.getByRole('tab', { name }));
  await flush();
}

async function refreshCurrent() {
  fireEvent.click(screen.getByRole('button', { name: /刷新当前页/ }));
  await flush();
}

function activePanel() {
  return screen.getByRole('tabpanel');
}

function resource(label: string, root: ParentNode = document) {
  const element = root.querySelector<HTMLElement>(`[data-resource="${label}"]`);
  expect(element, `resource status for ${label}`).not.toBeNull();
  return element!;
}

function timestamp(label: string, root: ParentNode = document) {
  return resource(label, root).querySelector('[title]')?.getAttribute('title');
}

async function expandMoreStats() {
  const toggle = within(activePanel()).getByRole('button', { name: '更多统计：累计用量、响应质量、模型与账号分布' });
  if (toggle.getAttribute('aria-expanded') !== 'true') {
    fireEvent.click(toggle);
    await flush();
  }
}

function statistic(label: string) {
  const node = within(activePanel()).getByText(label, { selector: '.ant-statistic-title' }).closest('.ant-statistic');
  expect(node).not.toBeNull();
  return node!;
}

function modelOrder() {
  return Array.from(activePanel().querySelectorAll('.jc-benchmarks tbody tr[data-row-key]'))
    .map(row => row.getAttribute('data-row-key'));
}

function requestCounts() {
  return {
    stats: vi.mocked(api.getStats).mock.calls.length,
    accounts: vi.mocked(api.listAccounts).mock.calls.length,
    logs: vi.mocked(api.getRecentLogs).mock.calls.length,
    costs: vi.mocked(api.getCosts).mock.calls.length,
    benchmarks: vi.mocked(api.getModelBenchmarks).mock.calls.length,
    capabilities: vi.mocked(api.getModelCapabilities).mock.calls.length,
    health: vi.mocked(api.getHealth).mock.calls.length,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(START);
  localStorage.clear();
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
  vi.stubGlobal('matchMedia', vi.fn((query: string) => ({
    matches: false, media: query, onchange: null, addListener: vi.fn(), removeListener: vi.fn(),
    addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
  })));
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  // rc-table measures pseudo-element scrollbars; jsdom only supports ordinary styles.
  const getComputedStyle = window.getComputedStyle.bind(window);
  vi.spyOn(window, 'getComputedStyle').mockImplementation(element => getComputedStyle(element));
  vi.spyOn(api, 'getStats').mockResolvedValue(stats);
  vi.spyOn(api, 'listAccounts').mockResolvedValue(accounts);
  vi.spyOn(api, 'getRecentLogs').mockResolvedValue({ logs, total: logs.length });
  vi.spyOn(api, 'getCosts').mockResolvedValue(costs);
  vi.spyOn(api, 'getModelBenchmarks').mockResolvedValue(benchmarks);
  vi.spyOn(api, 'getModelCapabilities').mockResolvedValue({ models: [], upstream_cap_ctx: 0, request_body_cap: 0, probed_at: '2026-09-09' });
  vi.spyOn(api, 'getHealth').mockResolvedValue({ status: 'ok', accounts: 1 });
  vi.spyOn(api, 'getRepoStars').mockResolvedValue({ github: 0, gitee: 0, repos: { github: 'https://github.com/variyaone/JoyCode2api-VABoost', gitee: 'https://gitee.com/variyaone/JoyCode2api-VABoost' } });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, 'visibilityState');
  localStorage.clear();
});

// Real Ant Design tables can exceed the 5s default on a busy Windows worker.
// This is a per-suite wall-clock budget, not a change to fake polling intervals.
describe('OperationsDetails resource lifecycle', { timeout: 15_000 }, () => {
  it('keeps real layout navigation and independent attribution without duplicating API reads', async () => {
    renderOperations({ realLayout: true });
    await flush();
    expect(screen.getByRole('heading', { name: '运行明细' })).toBeTruthy();
    expect(screen.getByRole('navigation', { name: '主导航' })).toBeTruthy();
    expect(screen.queryByRole('link', { name: '由 Variya 维护' })).toBeNull();
    expect(screen.getByRole('link', { name: /vibe-coding-labs/ })).toBeTruthy();
    expect(api.getRepoStars).toHaveBeenCalled();
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 1, benchmarks: 0, capabilities: 0, health: 1 });
    expect(screen.getByRole('link', { name: /176ca9d/ })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: '打开导航' }));
    await advance(200);
    expect(screen.getByRole('menu')).toBeTruthy();
    await tab('费用明细');
    expect(screen.getByRole('heading', { name: '费用明细' })).toBeTruthy();
    expect(api.getCosts).toHaveBeenCalledTimes(1);
  });
  it('shares the parent cost resource and currency with the real detail tables', async () => {
    renderOperations();
    await flush();
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 1, benchmarks: 0, capabilities: 0, health: 1 });
    expect(ownerCosts()).toEqual({ currency: 'CNY', data: costs });
    const originalStamp = timestamp('用量账本');
    await tab('费用明细');
    expect(screen.getByRole('tab', { name: '费用明细' }).getAttribute('aria-selected')).toBe('true');
    expect(within(activePanel()).getAllByText('¥7.20').length).toBeGreaterThan(0);
    expect(within(activePanel()).getAllByText('¥21.60').length).toBeGreaterThan(0);
    expect(timestamp('费用', activePanel())).toBe(originalStamp);
    expect(api.getCosts).toHaveBeenCalledTimes(1);

    fireEvent.click(within(activePanel()).getByText('美元 $'));
    await flush();
    expect(localStorage.getItem('jc_cost_currency')).toBe('USD');
    expect(ownerCosts()).toEqual({ currency: 'USD', data: costs });
    expect(within(activePanel()).getAllByText('$1.00').length).toBeGreaterThan(0);
    expect(within(activePanel()).getAllByText('$3.00').length).toBeGreaterThan(0);
    await tab('运行概览');
    expect(ownerCosts().currency).toBe('USD');
    expect(api.getCosts).toHaveBeenCalledTimes(1);

    await tab('费用明细');
    expect((within(activePanel()).getByRole('radio', { name: '美元 $' }) as HTMLInputElement).checked).toBe(true);
    await advance(5_000);
    const updatedCosts = { ...costs, rows: [{ ...costs.rows[0], amount_tenth_micro_usd: 40_000_000 }] };
    vi.mocked(api.getCosts).mockResolvedValue(updatedCosts);
    fireEvent.click(within(activePanel()).getByRole('button', { name: '刷新费用' }));
    await flush();
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 2, benchmarks: 0, capabilities: 0, health: 1 });
    expect(timestamp('费用', activePanel())).not.toBe(originalStamp);
    expect(timestamp('费用', activePanel())).toBe(timestamp('用量账本'));
    expect(ownerCosts()).toEqual({ currency: 'USD', data: updatedCosts });
    expect(within(activePanel()).getAllByText('$4.00').length).toBeGreaterThan(0);
    await tab('运行概览');
    expect(ownerCosts()).toEqual({ currency: 'USD', data: updatedCosts });
    await tab('费用明细');
    expect(within(activePanel()).getAllByText('$4.00').length).toBeGreaterThan(0);
    expect(api.getCosts).toHaveBeenCalledTimes(2);
  });

  it('loads benchmark/capability snapshots only on first model-tab visit, never polls them, and refreshes only the active resources', async () => {
    renderOperations();
    await flush();
    await tab('费用明细');
    await advance(30_000);
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 2, benchmarks: 0, capabilities: 0, health: 2 });
    await tab('模型参考');
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 2, benchmarks: 1, capabilities: 1, health: 2 });
    expect(screen.getByText('模型参考仅手动重读；代理连接继续定时检查。')).toBeTruthy();
    await advance(90_000);
    // The outer Usage report still owns/polls costs at 60s, 90s and 120s;
    // the inner static model snapshots must not poll along with it.
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 5, benchmarks: 1, capabilities: 1, health: 5 });
    expect(resource('公开评测', activePanel()).textContent).toContain('本地快照，不是实时探测');
    expect(resource('公开评测', activePanel()).textContent).not.toContain('数据可能过期');
    await refreshCurrent();
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 5, benchmarks: 2, capabilities: 2, health: 6 });
    await tab('运行概览');
    // Live resources catch up after being inactive, but the owner costs are fresh.
    expect(requestCounts()).toEqual({ stats: 2, accounts: 2, logs: 2, costs: 5, benchmarks: 2, capabilities: 2, health: 6 });
    await tab('模型参考');
    expect(api.getModelBenchmarks).toHaveBeenCalledTimes(2);
    expect(api.getModelCapabilities).toHaveBeenCalledTimes(2);
  });

  it('retains ModelBenchmarks search, ascending sort, and display mode across tab switches and manual refresh', async () => {
    renderOperations();
    await flush();
    await tab('模型参考');
    expect(modelOrder()).toEqual(['Atlas-high', 'Other-model', 'Atlas-low']);
    fireEvent.change(screen.getByRole('searchbox', { name: '筛选评测模型' }), { target: { value: 'atlas' } });
    fireEvent.click(within(activePanel()).getByText('从低到高'));
    await flush();
    expect(modelOrder()).toEqual(['Atlas-low', 'Atlas-high']);
    fireEvent.click(within(activePanel()).getByText('AA 指数明细'));
    await flush();
    expect(within(activePanel()).getByRole('columnheader', { name: 'JoyCode 模型' })).toBeTruthy();
    await tab('费用明细');
    await tab('运行概览');
    await tab('模型参考');
    expect((screen.getByRole('searchbox', { name: '筛选评测模型' }) as HTMLInputElement).value).toBe('atlas');
    expect((within(activePanel()).getByRole('radio', { name: '从低到高' }) as HTMLInputElement).checked).toBe(true);
    expect(within(activePanel()).getByRole('columnheader', { name: 'JoyCode 模型' })).toBeTruthy();
    expect(modelOrder()).toEqual(['Atlas-low', 'Atlas-high']);
    expect(api.getModelBenchmarks).toHaveBeenCalledTimes(1);
    await refreshCurrent();
    expect(modelOrder()).toEqual(['Atlas-low', 'Atlas-high']);
    expect((screen.getByRole('searchbox', { name: '筛选评测模型' }) as HTMLInputElement).value).toBe('atlas');
  });

  it('isolates first-load live-stat failures so costs, accounts, logs and models remain usable, with a scoped retry', async () => {
    vi.mocked(api.getStats).mockRejectedValue(new Error('stats unavailable'));
    renderOperations();
    await flush();
    expect(within(resource('运行统计')).getByText('运行统计读取失败')).toBeTruthy();
    expect(resource('运行统计').textContent).toContain('尚未读取');
    expect(timestamp('运行统计')).toBeUndefined();
    expect(within(activePanel()).queryByRole('button', { name: '更多统计：累计用量、响应质量、模型与账号分布' })).toBeNull();
    expect(ownerCosts()).toEqual({ currency: 'CNY', data: costs });
    expect(within(activePanel()).getByText('Retained account')).toBeTruthy();
    expect(within(activePanel()).getByText('Request-model-1')).toBeTruthy();
    await tab('费用明细');
    expect(within(activePanel()).getByText('Test cost ledger notice')).toBeTruthy();
    await tab('模型参考');
    expect(modelOrder()).toEqual(['Atlas-high', 'Other-model', 'Atlas-low']);
    await tab('运行概览');
    vi.mocked(api.getStats).mockResolvedValue(stats);
    const before = requestCounts();
    fireEvent.click(within(resource('运行统计')).getByRole('button', { name: /重\s*试/ }));
    await flush();
    expect(requestCounts()).toEqual({ ...before, stats: before.stats + 1 });
    expect(within(resource('运行统计')).queryByText('运行统计读取失败')).toBeNull();
    await expandMoreStats();
    expect(statistic('今日请求').textContent).toContain('125');
  });

  it('keeps successful live data and genuine successful timestamps on later rejections, then advances only the retried resource', async () => {
    renderOperations();
    await flush();
    const labels = ['运行统计', '账号', '请求日志', '用量账本', '代理连接'];
    const stamps = labels.map(label => timestamp(label));
    expect(stamps.every(Boolean)).toBe(true);
    vi.mocked(api.getStats).mockRejectedValue(new Error('stats unavailable'));
    vi.mocked(api.listAccounts).mockRejectedValue(new Error('accounts unavailable'));
    vi.mocked(api.getRecentLogs).mockRejectedValue(new Error('logs unavailable'));
    vi.mocked(api.getCosts).mockRejectedValue(new Error('costs unavailable'));
    vi.mocked(api.getHealth).mockRejectedValue(new Error('health unavailable'));
    await advance(30_000);
    for (const label of labels) {
      expect(resource(label).textContent).toContain('读取失败，保留上次数据');
      expect(resource(label).textContent).toContain('数据可能过期');
      expect(resource(label).textContent).toContain('30 秒前更新');
    }
    expect(labels.map(label => timestamp(label))).toEqual(stamps);
    await expandMoreStats();
    expect(statistic('今日请求').textContent).toContain('125');
    expect(ownerCosts()).toEqual({ currency: 'CNY', data: costs });
    expect(within(activePanel()).getByText('Retained account')).toBeTruthy();
    expect(within(activePanel()).getByText('Request-model-1')).toBeTruthy();
    await tab('费用明细');
    expect(within(activePanel()).getAllByText('¥7.20').length).toBeGreaterThan(0);
    expect(resource('费用', activePanel()).textContent).toContain('保留上次数据');
    await tab('运行概览');
    await advance(5_000);
    vi.mocked(api.getStats).mockResolvedValue({ ...stats, total_requests: 126 });
    fireEvent.click(within(resource('运行统计')).getByRole('button', { name: /重\s*试/ }));
    await flush();
    expect(statistic('今日请求').textContent).toContain('126');
    expect(timestamp('运行统计')).toBe(new Date(Date.now()).toLocaleString('zh-CN'));
    expect(timestamp('运行统计')).not.toBe(stamps[0]);
    expect(resource('运行统计').textContent).not.toContain('数据可能过期');
    expect(labels.slice(1).map(label => timestamp(label))).toEqual(stamps.slice(1));
  });

  it('retains a benchmark snapshot and its read time if manual refresh fails', async () => {
    renderOperations();
    await flush();
    await tab('模型参考');
    const originalStamp = timestamp('公开评测', activePanel());
    vi.mocked(api.getModelBenchmarks).mockRejectedValue(new Error('snapshot unavailable'));
    await advance(65_000);
    await refreshCurrent();
    expect(resource('公开评测', activePanel()).textContent).toContain('公开评测读取失败，保留上次数据');
    expect(resource('公开评测', activePanel()).textContent).toContain('1 分钟前更新');
    expect(timestamp('公开评测', activePanel())).toBe(originalStamp);
    expect(timestamp('历史能力记录', activePanel())).not.toBe(originalStamp);
    expect(modelOrder()).toEqual(['Atlas-high', 'Other-model', 'Atlas-low']);
    await tab('运行概览');
    await tab('模型参考');
    expect(api.getModelBenchmarks).toHaveBeenCalledTimes(2);
    expect(timestamp('公开评测', activePanel())).toBe(originalStamp);
  });

  it('turns off all scheduled current-resource/header refreshes without faking freshness; manual refresh remains tab-scoped', async () => {
    renderOperations();
    await flush();
    const originalStamp = timestamp('运行统计');
    fireEvent.click(screen.getByRole('switch', { name: '30 秒自动刷新' }));
    await flush();
    expect(screen.getByRole('switch', { name: '30 秒自动刷新' }).getAttribute('aria-checked')).toBe('false');
    expect(screen.getByText('自动刷新已关闭，可手动刷新。')).toBeTruthy();
    await advance(65_000);
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 1, benchmarks: 0, capabilities: 0, health: 1 });
    expect(timestamp('运行统计')).toBe(originalStamp);
    expect(resource('运行统计').textContent).toContain('1 分钟前更新（数据可能过期）');
    expect(resource('代理连接').textContent).toContain('1 分钟前更新（数据可能过期）');
    await tab('费用明细');
    expect(api.getCosts).toHaveBeenCalledTimes(1);
    await refreshCurrent();
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 2, benchmarks: 0, capabilities: 0, health: 2 });
    await tab('模型参考');
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 2, benchmarks: 1, capabilities: 1, health: 2 });
    await advance(90_000);
    await tab('运行概览');
    expect(requestCounts()).toEqual({ stats: 1, accounts: 1, logs: 1, costs: 2, benchmarks: 1, capabilities: 1, health: 2 });
    expect(timestamp('运行统计')).toBe(originalStamp);
    await refreshCurrent();
    expect(requestCounts()).toEqual({ stats: 2, accounts: 2, logs: 2, costs: 3, benchmarks: 1, capabilities: 1, health: 3 });
    await advance(30_000);
    expect(requestCounts()).toEqual({ stats: 2, accounts: 2, logs: 2, costs: 3, benchmarks: 1, capabilities: 1, health: 3 });
  });

  it('preserves the real recent-request table page across tab switches, polling and a failed refresh', async () => {
    renderOperations();
    await flush();
    const recent = screen.getByRole('region', { name: '最近请求' });
    fireEvent.click(within(recent).getByTitle('2'));
    await flush();
    expect(within(recent).getByText('Request-model-11')).toBeTruthy();
    expect(within(recent).queryByText('Request-model-1')).toBeNull();
    await tab('模型参考');
    await advance(31_000);
    vi.mocked(api.getRecentLogs).mockResolvedValue({ logs: logs.map(log => ({ ...log })), total: logs.length });
    await tab('运行概览');
    expect(api.getRecentLogs).toHaveBeenCalledTimes(2);
    expect(within(screen.getByRole('region', { name: '最近请求' })).getByText('Request-model-11')).toBeTruthy();
    expect(within(screen.getByRole('region', { name: '最近请求' })).queryByText('Request-model-1')).toBeNull();
    vi.mocked(api.getRecentLogs).mockRejectedValue(new Error('logs unavailable'));
    await refreshCurrent();
    expect(resource('请求日志').textContent).toContain('保留上次数据');
    expect(within(screen.getByRole('region', { name: '最近请求' })).getByText('Request-model-11')).toBeTruthy();
  });

  it('aborts a deactivated live request and ignores its late completion while keeping costs usable', async () => {
    const old = deferred<Stats>();
    vi.mocked(api.getStats).mockReturnValueOnce(old.promise);
    renderOperations({ autoRefresh: false });
    await flush();
    const oldSignal = vi.mocked(api.getStats).mock.calls[0][0]!;
    expect(oldSignal.aborted).toBe(false);
    await tab('费用明细');
    expect(oldSignal.aborted).toBe(true);
    expect(within(activePanel()).getAllByText('¥7.20').length).toBeGreaterThan(0);
    vi.mocked(api.getStats).mockResolvedValue({ ...stats, total_requests: 222 });
    await tab('运行概览');
    await expandMoreStats();
    expect(statistic('今日请求').textContent).toContain('222');
    const newStamp = timestamp('运行统计');
    await advance(5_000);
    await act(async () => { old.resolve({ ...stats, total_requests: 999 }); });
    expect(statistic('今日请求').textContent).toContain('222');
    expect(statistic('今日请求').textContent).not.toContain('999');
    expect(timestamp('运行统计')).toBe(newStamp);
  });

  it('cleans up all pending operations, parent-cost and health requests and polling on unmount', async () => {
    const pending = deferred<Stats>();
    const pendingCosts = deferred<CostSnapshot>();
    const pendingHealth = deferred<Awaited<ReturnType<typeof api.getHealth>>>();
    vi.mocked(api.getStats).mockReturnValue(pending.promise);
    vi.mocked(api.getCosts).mockReturnValue(pendingCosts.promise);
    vi.mocked(api.getHealth).mockReturnValue(pendingHealth.promise);
    const { unmount } = renderOperations();
    await flush();
    const statsSignal = vi.mocked(api.getStats).mock.calls[0][0]!;
    const costsSignal = vi.mocked(api.getCosts).mock.calls[0][0]!;
    const healthSignal = vi.mocked(api.getHealth).mock.calls[0][0]!;
    const before = requestCounts();
    expect([statsSignal, costsSignal, healthSignal].every(signal => !signal.aborted)).toBe(true);
    unmount();
    expect(statsSignal.aborted).toBe(true);
    expect(costsSignal.aborted).toBe(true);
    expect(healthSignal.aborted).toBe(true);
    await act(async () => { pending.resolve(stats); pendingCosts.resolve(costs); pendingHealth.reject(new Error('late failure')); });
    await advance(120_000);
    expect(requestCounts()).toEqual(before);
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe('OperationsDetails with the real MainLayout header', { timeout: 15_000 }, () => {
  it('shares the automatic-refresh switch with health, ages a paused status, and distinguishes failed from fresh health', async () => {
    vi.mocked(api.getHealth).mockResolvedValue({ status: 'ok', accounts: 7 });
    renderOperations({ realLayout: true });
    await flush();
    const header = screen.getByRole('region', { name: '代理连接状态' });
    expect(within(header).getByText('代理可连接')).toBeTruthy();
    expect(within(header).getByText('已配置 7 个账号')).toBeTruthy();
    expect(api.getHealth).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('switch', { name: '30 秒自动刷新' }));
    await flush();
    expect(localStorage.getItem('jc_auto_refresh')).toBe('false');
    await advance(65_000);
    expect(api.getHealth).toHaveBeenCalledTimes(1);
    expect(within(header).getByText('代理状态待更新')).toBeTruthy();
    expect(within(header).getByText('已配置 7 个账号（上次记录）')).toBeTruthy();
    expect(within(header).queryByText('代理可连接')).toBeNull();

    await tab('费用明细');
    const pendingHealth = deferred<Awaited<ReturnType<typeof api.getHealth>>>();
    vi.mocked(api.getHealth).mockReturnValueOnce(pendingHealth.promise);
    await refreshCurrent();
    const refreshButton = screen.getByRole('button', { name: /刷新当前页/ });
    expect(refreshButton.classList.contains('ant-btn-loading')).toBe(true);
    expect(api.getCosts).toHaveBeenCalledTimes(2);
    expect(api.getStats).toHaveBeenCalledTimes(1);
    await act(async () => { pendingHealth.reject(new Error('connection lost')); });
    expect(screen.getByRole('button', { name: /刷新当前页/ }).classList.contains('ant-btn-loading')).toBe(false);
    expect(within(header).getByText('状态获取失败')).toBeTruthy();
    expect(within(header).getByText('已配置 7 个账号（上次记录）')).toBeTruthy();
    expect(resource('代理连接').textContent).toContain('保留上次数据');

    vi.mocked(api.getHealth).mockResolvedValue({ status: 'ok', accounts: 8 });
    await refreshCurrent();
    expect(within(header).getByText('代理可连接')).toBeTruthy();
    expect(within(header).getByText('已配置 8 个账号')).toBeTruthy();
    expect(localStorage.getItem('jc_auto_refresh')).toBe('false');
  });

  it('does not label an unhealthy response or failed initial health read as a usable connection', async () => {
    localStorage.setItem('jc_auto_refresh', 'false');
    vi.mocked(api.getHealth).mockResolvedValue({ status: 'error', accounts: 9 });
    renderOperations({ realLayout: true });
    await flush();
    const header = screen.getByRole('region', { name: '代理连接状态' });
    expect(within(header).getByText('状态获取失败')).toBeTruthy();
    expect(within(header).getByText('账号数量待读取')).toBeTruthy();
    expect(within(header).queryByText('代理可连接')).toBeNull();
    expect(resource('代理连接').textContent).toContain('尚未读取');
    expect(timestamp('代理连接')).toBeUndefined();
    await expandMoreStats();
    expect(statistic('今日请求').textContent).toContain('125');
    await tab('模型参考');
    expect(modelOrder()).toHaveLength(3);
    vi.mocked(api.getHealth).mockResolvedValue({ status: 'ok', accounts: 2 });
    await refreshCurrent();
    expect(within(header).getByText('代理可连接')).toBeTruthy();
    expect(within(header).getByText('已配置 2 个账号')).toBeTruthy();
  });
});
