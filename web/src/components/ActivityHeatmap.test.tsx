import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { ConfigProvider } from 'antd';
import ActivityHeatmap from './ActivityHeatmap';
import { api } from '../api';
import type { CostSnapshot, UsageActivity } from '../api';
import { usageReport } from '../utils/usageReport';

const snapshot: CostSnapshot = { currency: 'USD', price_version: 'test', collected_at: '2026-09-11', today: '2026-09-11', timezone: 'CST +08:00', coverage_start: '2026-08-01', notice: '', rates: [], rows: [] };
function response(from: string, through: string): UsageActivity {
  return { from, through, today: '2026-09-11', timezone: 'CST +08:00', generated_at: '2026-09-11T12:30:00+08:00', source: 'request_logs', days: [
    { date: through, coverage: 'matched', raw_requests: 3, ledger_requests: 3, hours: [{ hour: 10, requests: 3, input_tokens: 90, output_tokens: 10 }] },
  ] };
}
beforeEach(() => {
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  vi.stubGlobal('matchMedia', vi.fn().mockImplementation(query => ({ matches: false, media: query, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {} })));
  const computed = window.getComputedStyle.bind(window);
  vi.spyOn(window, 'getComputedStyle').mockImplementation(element => computed(element));
  vi.spyOn(api, 'getUsageActivity').mockImplementation(async (from, through) => response(from, through));
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); Reflect.deleteProperty(document, 'visibilityState'); });
const mount = (range: 'all' | '7d' | '30d' = '7d') => render(<ConfigProvider theme={{ token: { motion: false } }}><ActivityHeatmap report={usageReport(snapshot, range)} autoRefresh={false} refreshKey={0} /></ConfigProvider>);

describe('date by hour activity heatmap', { timeout: 20000 }, () => {
  it('uses 24 hours for each day and shows missing, zero and future differently', async () => {
    const { container } = mount();
    await waitFor(() => expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(7 * 24));
    expect(api.getUsageActivity).toHaveBeenCalledWith('2026-09-05', '2026-09-11', expect.any(AbortSignal));
    expect(screen.getByRole('button', { name: /2026-09-11 10:00–11:00.*3 次请求/ })).toBeTruthy();
    expect(screen.getByRole('button', { name: /2026-09-11 09:00–10:00/ }).getAttribute('data-state')).toBe('zero');
    expect(screen.getByRole('button', { name: /2026-09-05 09:00–10:00/ }).getAttribute('data-state')).toBe('unknown');
    expect(screen.getByRole('button', { name: /2026-09-11 13:00–14:00/ }).getAttribute('data-state')).toBe('future');
  });
  it('keeps All explicit and limits each page to 31 x 24 cells', async () => {
    const { container } = mount('all');
    await waitFor(() => expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(31 * 24));
    expect(api.getUsageActivity).toHaveBeenCalledWith('2026-08-12', '2026-09-11', expect.any(AbortSignal));
    fireEvent.click(screen.getByRole('button', { name: '更早日期' }));
    await waitFor(() => expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(11 * 24));
    expect(api.getUsageActivity).toHaveBeenLastCalledWith('2026-08-01', '2026-08-11', expect.any(AbortSignal));
    expect(screen.getByText(/所选 42 天/)).toBeTruthy();
  });
  it('uses a single tab stop with keyboard navigation and readable cell details', async () => {
    const { container } = mount();
    const first = await screen.findByRole('button', { name: /2026-09-05 00:00–01:00/ });
    act(() => first.focus());
    fireEvent.keyDown(first, { key: 'ArrowRight' });
    expect(document.activeElement?.getAttribute('aria-label')).toContain('01:00–02:00');
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' });
    expect(document.activeElement?.getAttribute('aria-label')).toContain('2026-09-06 01:00–02:00');
    expect(container.querySelectorAll('.jc-hour-cell[tabindex="0"]')).toHaveLength(1);
    expect(container.querySelector('.jc-hourly-readout')?.textContent).toContain('2026-09-06');
    fireEvent.click(screen.getByRole('radio', { name: 'Token' }));
    expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(168);
  });
  it('provides a table view without fake counts for absent hours', async () => {
    mount(); await screen.findByRole('button', { name: /2026-09-05 00:00–01:00/ });
    fireEvent.click(screen.getByRole('button', { name: '查看数据表' }));
    expect(screen.getByRole('columnheader', { name: '时段' })).toBeTruthy();
    expect(screen.getAllByText('记录不足').length).toBeGreaterThan(0);
  });
  it('ignores old date-range completion when the page changes', async () => {
    let resolve!: (value: UsageActivity) => void;
    vi.mocked(api.getUsageActivity).mockReturnValueOnce(new Promise(done => { resolve = done; }));
    const { container } = mount('all');
    await waitFor(() => expect(api.getUsageActivity).toHaveBeenCalledTimes(1));
    const oldSignal = vi.mocked(api.getUsageActivity).mock.calls[0][2]!;
    fireEvent.click(screen.getByRole('button', { name: '更早日期' }));
    await waitFor(() => expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(264));
    expect(oldSignal.aborted).toBe(true);
    await act(async () => resolve(response('2026-08-12', '2026-09-11')));
    expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(264);
    expect(container.querySelector('.jc-hour-cell')?.getAttribute('aria-label')).toContain('2026-08-01');
  });
  it('rejects mismatched range data rather than displaying another period', async () => {
    vi.mocked(api.getUsageActivity).mockResolvedValueOnce(response('2026-08-01', '2026-08-02'));
    const { container } = mount();
    await screen.findByText('时段记录读取失败');
    expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(0);
    fireEvent.click(within(container.querySelector('[data-resource="时段记录"]')!).getByRole('button', { name: /重\s*试/ }));
    await waitFor(() => expect(container.querySelectorAll('.jc-hour-cell')).toHaveLength(168));
  });
});
