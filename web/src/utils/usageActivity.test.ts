import { describe, expect, it } from 'vitest';
import type { UsageActivity } from '../api';
import { activityCellLabel, activityRows, hourLabel } from './usageActivity';
const data: UsageActivity = {
  from: '2026-09-10', through: '2026-09-12', today: '2026-09-11', generated_at: '2026-09-11T13:45:00+08:00', timezone: 'CST +08:00', source: 'request_logs',
  days: [
    { date: '2026-09-10', coverage: 'partial', raw_requests: 1, ledger_requests: 4, hours: [{ hour: 8, requests: 1, input_tokens: 10, output_tokens: 2 }] },
    { date: '2026-09-11', coverage: 'matched', raw_requests: 2, ledger_requests: 2, hours: [{ hour: 11, requests: 2, input_tokens: 8, output_tokens: 3 }] },
  ],
};
describe('date × hour evidence', () => {
  it('creates exactly 24 hours per date without spreading day totals', () => {
    const rows = activityRows(data);
    expect(rows).toHaveLength(3);
    expect(rows.every(row => row.cells.length === 24)).toBe(true);
    expect(rows[0].cells[8]).toMatchObject({ state: 'partial', requests: 1, input: 10, output: 2, tokens: 12 });
    expect(rows[0].cells[7]).toMatchObject({ state: 'unknown', requests: null, tokens: null });
    expect(rows[1].cells[11]).toMatchObject({ state: 'recorded', requests: 2, tokens: 11 });
    expect(rows[1].cells[10]).toMatchObject({ state: 'zero', requests: 0, tokens: 0 });
  });
  it('uses server local hour, not the browser timezone, and never invents future zeros', () => {
    const rows = activityRows(data);
    expect(rows[1].cells[13].state).toBe('zero');
    expect(rows[1].cells[14]).toMatchObject({ state: 'future', requests: null, tokens: null });
    expect(rows[2].cells.every(cell => cell.state === 'future')).toBe(true);
  });
  it('keeps insufficient/missing hour data unknown', () => {
    const rows = activityRows({ ...data, days: [] });
    expect(rows[0].cells.every(cell => cell.state === 'unknown')).toBe(true);
    expect(activityCellLabel(rows[0].cells[0])).toContain('不能按 0 计算');
  });
  it('labels both day and one-hour interval with truthful evidence caveats', () => {
    const rows = activityRows(data);
    expect(hourLabel(23)).toBe('23:00–24:00');
    expect(activityCellLabel(rows[0].cells[8])).toContain('2026-09-10 08:00–09:00');
    expect(activityCellLabel(rows[0].cells[8])).toContain('现存日志，不完整');
    expect(activityCellLabel(rows[1].cells[14])).toContain('尚未发生');
  });
});
