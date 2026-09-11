import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, within, waitFor } from '@testing-library/react';
import { ConfigProvider } from 'antd';
import { MemoryRouter, Outlet, Route, Routes } from 'react-router-dom';
import Dashboard from './Dashboard';
import { api } from '../api';
import type { CostSnapshot } from '../api';

vi.mock('recharts', () => {
  const Chart = () => null;
  return { ResponsiveContainer: Chart, BarChart: Chart, Bar: Chart, CartesianGrid: Chart, XAxis: Chart, YAxis: Chart, Tooltip: Chart, AreaChart: Chart, Area: Chart, Line: Chart, ComposedChart: Chart, Legend: Chart };
});
const snapshot: CostSnapshot = { currency: 'USD', price_version: 'test', collected_at: '2026-09-11', today: '2026-09-11', timezone: 'CST +08:00', coverage_start: '2026-08-01', notice: 'Recorded ledger, not sessions', rates: [], rows: [
  { day: '2026-08-01', model: 'Older', price_version: 'test', requests: 1, input_tokens: 800, output_tokens: 100, missing_usage: 0, input_rate: null, output_rate: null, amount_tenth_micro_usd: null },
  { day: '2026-09-05', model: 'Recent', price_version: 'test', requests: 4, input_tokens: 70, output_tokens: 30, missing_usage: 0, input_rate: null, output_rate: null, amount_tenth_micro_usd: null },
  { day: '2026-09-11', model: 'Recent', price_version: 'test', requests: 3, input_tokens: 150, output_tokens: 50, missing_usage: 0, input_rate: null, output_rate: null, amount_tenth_micro_usd: null },
] };
const health = { error: '', lastSuccessAt: Date.now(), refreshing: false, stale: false, refresh: vi.fn(async () => {}) };
function mount() {
  return render(<ConfigProvider theme={{ token: { motion: false } }}><MemoryRouter><Routes><Route element={<Outlet context={{ autoRefresh: false, setAutoRefresh: vi.fn(), health }} />}><Route path="/" element={<Dashboard />} /></Route></Routes></MemoryRouter></ConfigProvider>);
}
const metric = (label: string) => screen.getByText(label, { selector: 'h3' }).closest('article')!;
async function ready() { await screen.findByText('请求次数', { selector: 'h3' }); }
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  vi.stubGlobal('matchMedia', vi.fn().mockImplementation(query => ({ matches: false, media: query, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {}, dispatchEvent: () => false })));
  const computed = window.getComputedStyle.bind(window);
  vi.spyOn(window, 'getComputedStyle').mockImplementation(element => computed(element));
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
  vi.spyOn(api, 'getCosts').mockResolvedValue(snapshot);
  vi.spyOn(api, 'getUsageActivity').mockImplementation(async (from, through) => ({ from, through, today: snapshot.today, timezone: snapshot.timezone, generated_at: '2026-09-11T18:00:00+08:00', source: 'request_logs', days: [] }));
  vi.spyOn(api, 'getStats'); vi.spyOn(api, 'listAccounts'); vi.spyOn(api, 'getRecentLogs');
  localStorage.clear();
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); Reflect.deleteProperty(document, 'visibilityState'); });

describe('Usage report integration', { timeout: 20000 }, () => {
  it('shows four consumer metrics and does not preload operational APIs', async () => {
    mount(); await ready();
    expect(document.querySelectorAll('.jc-usage-metric')).toHaveLength(4);
    expect(metric('请求次数').textContent).toContain('8');
    expect(metric('总 Token').textContent).toContain('输入 1.0K / 输出 180');
    expect(metric('平均每次 Token').textContent).toContain('150');
    expect(metric('参考费用').textContent).toContain('暂无法估算');
    expect(screen.queryByText('当前连续使用')).toBeNull();
    expect(screen.queryByText('Sessions')).toBeNull();
    expect(screen.queryByText('Peak hour')).toBeNull();
    expect(metric('总 Token').textContent).toContain('1.2K');
    expect(screen.getByRole('region', { name: '每日消耗趋势' })).toBeTruthy();
    expect(screen.getByRole('region', { name: '模型消耗排行' })).toBeTruthy();
    expect(api.getStats).not.toHaveBeenCalled(); expect(api.listAccounts).not.toHaveBeenCalled(); expect(api.getRecentLogs).not.toHaveBeenCalled();
  });
  it('uses one selected range for overview, calendar, models and daily table', async () => {
    mount(); await ready();
    fireEvent.click(screen.getByRole('radio', { name: '7d' }));
    await waitFor(() => expect(document.querySelectorAll('.jc-hour-cell')).toHaveLength(7 * 24));
    expect(metric('总 Token').textContent).toContain('300');
    expect(metric('请求次数').textContent).toContain('所选范围 2 个有记录日');
    fireEvent.click(screen.getByRole('tab', { name: 'Models' }));
    expect(document.querySelector('.jc-usage-model-list')?.textContent).toContain('Recent');
    expect(document.querySelector('.jc-usage-model-list')?.textContent).not.toContain('Older');
    expect(document.querySelector('.jc-model-usage-detail')?.textContent).toContain('输入 220');
    fireEvent.click(screen.getByRole('tab', { name: 'Overview' }));
    fireEvent.click(within(screen.getByRole('radiogroup', { name: '每日 Token 呈现方式' })).getByRole('radio', { name: '每日明细' }));
    expect(document.querySelectorAll('tbody tr[data-row-key]')).toHaveLength(7);
    fireEvent.click(screen.getByRole('radio', { name: '30d' }));
    fireEvent.click(screen.getByRole('tab', { name: 'Overview' }));
    await waitFor(() => expect(document.querySelectorAll('.jc-hour-cell')).toHaveLength(30 * 24));
    expect(metric('总 Token').textContent).toContain('300');
    fireEvent.click(screen.getByRole('radio', { name: 'All' }));
    await waitFor(() => expect(document.querySelectorAll('.jc-hour-cell')).toHaveLength(31 * 24));
    expect(screen.getByText(/所选 42 天/)).toBeTruthy();
    expect(metric('总 Token').textContent).toContain('1.2K');
    expect(api.getCosts).toHaveBeenCalledTimes(1);
  });
  it('preserves genuine prior data and marks failed refresh, then recovers', async () => {
    mount(); await ready();
    vi.mocked(api.getCosts).mockRejectedValueOnce(new Error('test outage'));
    fireEvent.click(screen.getByRole('button', { name: '刷新用量报告' }));
    await screen.findByText('用量账本读取失败，保留上次数据');
    expect(metric('总 Token').textContent).toContain('1.2K');
    expect(document.querySelector('[data-resource="用量账本"]')?.textContent).toContain('数据可能过期');
    fireEvent.click(screen.getByRole('button', { name: '刷新用量报告' }));
    await act(async () => { await Promise.resolve(); });
    expect(screen.queryByText('用量账本读取失败，保留上次数据')).toBeNull();
  });
  it('keeps first-load failure retryable rather than inventing zeros', async () => {
    vi.mocked(api.getCosts).mockRejectedValueOnce(new Error('offline'));
    mount(); await screen.findByText('用量账本读取失败');
    expect(document.querySelectorAll('.jc-usage-metric')).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: '刷新用量报告' }));
    await ready(); expect(metric('总 Token').textContent).toContain('1.2K');
  });
});
