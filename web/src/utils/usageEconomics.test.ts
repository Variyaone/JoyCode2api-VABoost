import { describe, expect, it } from 'vitest';
import type { CostRow, CostSnapshot } from '../api';
import { usageReport } from './usageReport';
import { usageEconomics } from './usageEconomics';
const row = (day: string, model: string, amount: number | null, input = 100): CostRow => ({ day, model, amount_tenth_micro_usd: amount, price_version: 'v1', requests: 1, input_tokens: input, output_tokens: 10, missing_usage: 0, input_rate: amount === null ? null : 1, output_rate: amount === null ? null : 2 });
const base: CostSnapshot = { currency: 'USD', price_version: 'v1', collected_at: '2026-09-01', today: '2026-09-12', timezone: 'CST +08:00', coverage_start: '2026-08-01', notice: '', rates: [], rows: [] };
describe('consumer cost report', () => {
  it('uses exactly the same selected dates for daily and model costs, preserving unknown prices', () => {
    const data = { ...base, rows: [row('2026-08-01','old',90_000_000), row('2026-09-06','A',10_000_000), row('2026-09-12','A',20_000_000), row('2026-09-12','B',null)] };
    const report = usageReport(data,'7d'); const result = usageEconomics(report,data);
    expect(result.total.requests).toBe(3); expect(result.amount).toBe(30_000_000); expect(result.total.unpriced).toBe(1);
    expect(result.models.has('old')).toBe(false); expect(result.models.get('A')?.amount).toBe(30_000_000); expect(result.models.get('B')?.amount).toBeNull();
    expect(result.days).toHaveLength(7); expect(result.days[0].amount).toBe(10_000_000); expect(result.days[6].amount).toBe(20_000_000);
    expect(result.days.reduce((sum,day)=>sum+(day.amount??0),0)).toBe(result.amount);
  });
  it('does not fabricate zero costs for dates before recorded coverage or entirely unpriced usage', () => {
    const data = { ...base, coverage_start: '2026-09-11', rows: [row('2026-09-11','B',null)] };
    const result = usageEconomics(usageReport(data,'7d'),data);
    expect(result.amount).toBeNull(); expect(result.days[0].amount).toBeNull(); expect(result.days[5].amount).toBeNull(); expect(result.days[6].amount).toBe(0);
  });
  it('keeps missing-use-only estimates unknown instead of announcing free usage', () => {
    const r = { ...row('2026-09-12','A',0,0), output_tokens: 0, missing_usage: 1 };
    const data = { ...base, rows: [r] };
    const result = usageEconomics(usageReport(data,'7d'),data);
    expect(result.amount).toBeNull(); expect(result.total.missing).toBe(1);
  });
});
