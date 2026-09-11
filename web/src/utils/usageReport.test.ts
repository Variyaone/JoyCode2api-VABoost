import { afterEach, describe, expect, it, vi } from 'vitest';
import type { CostRow, CostSnapshot } from '../api';
import { calendarDay, calendarTime, heatLevel, usageReport } from './usageReport';
import type { UsageRange } from './usageReport';

function row(day: string, overrides: Partial<CostRow> = {}): CostRow {
  return {
    day, model: 'Atlas', price_version: 'price-1', requests: 1,
    input_tokens: 10, output_tokens: 5, missing_usage: 0,
    input_rate: 1, output_rate: 2, amount_tenth_micro_usd: 20,
    ...overrides,
  };
}

function snapshot(overrides: Partial<CostSnapshot> = {}): CostSnapshot {
  return {
    today: '2026-09-11', timezone: 'Asia/Shanghai', coverage_start: '2026-09-01',
    collected_at: '2026-09-11T00:00:00Z', currency: 'USD', price_version: 'price-1',
    notice: 'Recorded usage only; missing dates do not prove inactivity.', rates: [], rows: [],
    ...overrides,
  };
}

afterEach(() => { vi.useRealTimers(); vi.unstubAllEnvs(); });

describe('usage report calendar arithmetic', () => {
  it.each([
    ['America/New_York', '2026-03-10', ['2026-03-04', '2026-03-05', '2026-03-06', '2026-03-07', '2026-03-08', '2026-03-09', '2026-03-10']],
    ['America/New_York', '2026-11-03', ['2026-10-28', '2026-10-29', '2026-10-30', '2026-10-31', '2026-11-01', '2026-11-02', '2026-11-03']],
    ['Europe/Berlin', '2026-03-31', ['2026-03-25', '2026-03-26', '2026-03-27', '2026-03-28', '2026-03-29', '2026-03-30', '2026-03-31']],
    ['Europe/Berlin', '2026-10-27', ['2026-10-21', '2026-10-22', '2026-10-23', '2026-10-24', '2026-10-25', '2026-10-26', '2026-10-27']],
    ['Asia/Shanghai', '2026-01-03', ['2025-12-28', '2025-12-29', '2025-12-30', '2025-12-31', '2026-01-01', '2026-01-02', '2026-01-03']],
    ['Pacific/Auckland', '2024-03-02', ['2024-02-25', '2024-02-26', '2024-02-27', '2024-02-28', '2024-02-29', '2024-03-01', '2024-03-02']],
    ['Asia/Shanghai', '2025-03-02', ['2025-02-24', '2025-02-25', '2025-02-26', '2025-02-27', '2025-02-28', '2025-03-01', '2025-03-02']],
  ])('enumerates seven inclusive server dates in %s through %s', (timezone, today, dates) => {
    // Deliberately disagree with both the server date and the server timezone.
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2035-01-01T23:59:59Z'));
    vi.stubEnv('TZ', 'Pacific/Honolulu');
    const result = usageReport(snapshot({ today, timezone, coverage_start: dates[0], rows: dates.map(day => row(day)) }), '7d');
    expect(result.days.map(day => day.date)).toEqual(dates);
    expect(result).toMatchObject({ start: dates[0], end: today, timezone, activeDays: 7, current: 7, longest: 7, tokens: 105 });
  });

  it.each([
    ['2026-01-03', '2025-12-05'],
    ['2024-03-01', '2024-02-01'],
    ['2025-03-01', '2025-01-31'],
    ['2026-03-15', '2026-02-14'],
    ['2026-11-05', '2026-10-07'],
  ])('enumerates exactly 30 inclusive dates ending %s, starting %s', (today, start) => {
    const result = usageReport(snapshot({ today, coverage_start: '2020-01-01', timezone: 'America/New_York' }), '30d');
    expect(result.start).toBe(start);
    expect(result.end).toBe(today);
    expect(result.days).toHaveLength(30);
    expect(new Set(result.days.map(day => day.date)).size).toBe(30);
    result.days.slice(1).forEach((day, index) => {
      expect(calendarTime(day.date) - calendarTime(result.days[index].date)).toBe(86_400_000);
    });
  });

  it.each([
    ['7d', '2026-09-05', '2026-09-04', 7],
    ['30d', '2026-08-13', '2026-08-12', 30],
  ] as const)('includes both %s endpoints and excludes older/future rows everywhere', (range, first, before, count) => {
    const result = usageReport(snapshot({ coverage_start: '2026-08-01', rows: [
      row(before, { model: 'Too-old', requests: 100, input_tokens: 100_000 }),
      row(first, { model: 'First', requests: 2 }),
      row('2026-09-11', { model: 'Last', requests: 3 }),
      row('2026-09-12', { model: 'Future', requests: 100, input_tokens: 100_000 }),
    ] }), range);
    expect(result.days).toHaveLength(count);
    expect(result).toMatchObject({ start: first, end: '2026-09-11', tokens: 30, requests: 5, activeDays: 2, current: 1, longest: 1, favorite: 'Last' });
    expect(result.models.map(model => model.model)).toEqual(['First', 'Last']);
    expect(result.days[0].requests).toBe(2);
    expect(result.days.at(-1)?.requests).toBe(3);
  });

  it('starts All at the ledger coverage date, including a timestamp coverage marker', () => {
    const result = usageReport(snapshot({ coverage_start: '2025-12-30T15:30:00+08:00', today: '2026-01-02', rows: [row('2026-01-01')] }), 'all');
    expect(result.days.map(day => day.date)).toEqual(['2025-12-30', '2025-12-31', '2026-01-01', '2026-01-02']);
    expect(result.coverageStart).toBe('2025-12-30T15:30:00+08:00');
  });

  it('uses earliest recorded date when the coverage marker is absent, not row order', () => {
    const result = usageReport(snapshot({ today: '2026-01-02', coverage_start: '', rows: [row('2026-01-02'), row('2025-12-31'), row('2026-01-05')] }), 'all');
    expect(result.days.map(day => [day.date, day.covered])).toEqual([
      ['2025-12-31', true], ['2026-01-01', false], ['2026-01-02', true],
    ]);
    expect(result).toMatchObject({ start: '2025-12-31', tokens: 30, requests: 2, partial: true });
  });

  it.each(['2024-02-29', '2000-02-29', '2026-12-31', '2027-01-01'])('round trips a valid calendar date %s', day => {
    expect(calendarDay(calendarTime(day))).toBe(day);
  });

  it.each(['2025-02-29', '1900-02-29', '2026-04-31', '2026-13-01', '2026-00-01', '2026-01-00', '2026-1-01', '2026-01-01T00:00:00Z', 'invalid', ''])('rejects invalid or noncanonical calendar date %s', day => {
    expect(() => calendarTime(day)).toThrow('Invalid calendar date');
  });

  it.each([
    { today: '2026-02-30' },
    { coverage_start: 'not-a-date' },
    { rows: [row('2026-02-30')] },
  ])('fails safely rather than silently normalizing invalid ledger dates: %j', overrides => {
    expect(() => usageReport(snapshot(overrides), 'all')).toThrow('Invalid calendar date');
  });
});

describe('recorded coverage and streaks', () => {
  it.each(['all', '7d', '30d'] as UsageRange[])('returns honest unknown coverage and no invented models for an empty %s ledger', range => {
    const result = usageReport(snapshot({ coverage_start: '' }), range);
    expect(result).toMatchObject({ tokens: 0, requests: 0, missing: 0, activeDays: 0, current: 0, longest: 0, favorite: null, hasCoverage: false, partial: true, models: [] });
    expect(result.days).toHaveLength(range === 'all' ? 1 : range === '7d' ? 7 : 30);
    expect(result.days.every(day => !day.covered && day.tokens === 0 && day.requests === 0)).toBe(true);
  });

  it('treats empty days within known coverage as zero recorded activity, not proof no calls occurred', () => {
    const result = usageReport(snapshot({ coverage_start: '2026-09-09' }), 'all');
    expect(result.days).toEqual(['2026-09-09', '2026-09-10', '2026-09-11'].map(date => ({ date, input: 0, output: 0, tokens: 0, requests: 0, missing: 0, covered: true })));
    expect(result).toMatchObject({ hasCoverage: true, partial: false, activeDays: 0, current: 0, longest: 0, favorite: null });
  });

  it('keeps dates preceding coverage distinct from zero recorded days', () => {
    const result = usageReport(snapshot({ coverage_start: '2026-09-09', rows: [row('2026-09-10')] }), '7d');
    expect(result.days.map(day => day.covered)).toEqual([false, false, false, false, true, true, true]);
    expect(result).toMatchObject({ start: '2026-09-05', hasCoverage: true, partial: true, tokens: 15, activeDays: 1, current: 1, longest: 1 });
  });

  it('does not backfill unknown coverage just because isolated dates have records', () => {
    const result = usageReport(snapshot({ coverage_start: '', rows: [row('2026-09-07'), row('2026-09-09')] }), '7d');
    expect(result.days.filter(day => day.covered).map(day => day.date)).toEqual(['2026-09-07', '2026-09-09']);
    expect(result).toMatchObject({ activeDays: 2, longest: 1, current: 0, partial: true });
  });

  it.each([
    ['today is active', ['2026-09-08', '2026-09-09', '2026-09-10', '2026-09-11'], 4, 4],
    ['yesterday is the latest recorded active day', ['2026-09-08', '2026-09-09', '2026-09-10'], 3, 3],
    ['latest recorded activity is older than yesterday', ['2026-09-07', '2026-09-08', '2026-09-09'], 0, 3],
    ['a recorded-day gap breaks the run', ['2026-09-05', '2026-09-06', '2026-09-07', '2026-09-09', '2026-09-11'], 1, 3],
  ] as const)('calculates current and longest streak when %s', (_label, dates, current, longest) => {
    const result = usageReport(snapshot({ rows: dates.map(day => row(day)) }), 'all');
    expect(result).toMatchObject({ activeDays: dates.length, current, longest });
  });

  it('clips current and longest streaks to the selected dates, never borrowing older active days', () => {
    const rows = Array.from({ length: 40 }, (_, i) => row(calendarDay(calendarTime('2026-08-03') + i * 86_400_000)));
    const data = snapshot({ coverage_start: '2026-08-03', rows });
    expect(usageReport(data, 'all')).toMatchObject({ activeDays: 40, longest: 40, current: 40 });
    expect(usageReport(data, '30d')).toMatchObject({ activeDays: 30, longest: 30, current: 30 });
    expect(usageReport(data, '7d')).toMatchObject({ activeDays: 7, longest: 7, current: 7 });
  });

  it('counts requests without token usage as active without fabricating tokens', () => {
    const result = usageReport(snapshot({ rows: [
      row('2026-09-10', { input_tokens: 0, output_tokens: 0, requests: 4, missing_usage: 4 }),
      row('2026-09-11', { input_tokens: 0, output_tokens: 0, requests: 2, missing_usage: 2 }),
    ] }), '7d');
    expect(result).toMatchObject({ tokens: 0, requests: 6, missing: 6, activeDays: 2, current: 2, longest: 2, favorite: 'Atlas' });
    expect(result.models[0]).toMatchObject({ tokens: 0, requests: 6, missing: 6, share: 0 });
    expect(result.days.every(day => day.tokens === 0)).toBe(true);
  });

  it('does not infer active days from tokens alone when recorded request count is zero', () => {
    const result = usageReport(snapshot({ rows: [row('2026-09-11', { requests: 0 })] }), '7d');
    expect(result).toMatchObject({ tokens: 15, requests: 0, activeDays: 0, current: 0, longest: 0, favorite: null });
  });
});

describe('model and token aggregation', () => {
  it('sums every price version into the exact day/model and leaves input data unchanged', () => {
    const data = snapshot({ rows: [
      row('2026-09-10', { model: 'Atlas', price_version: 'old', requests: 2, input_tokens: 100, output_tokens: 20 }),
      row('2026-09-10', { model: 'Atlas', price_version: 'new', requests: 3, input_tokens: 50, output_tokens: 30, missing_usage: 1 }),
      row('2026-09-11', { model: 'Atlas', price_version: 'new', requests: 1, input_tokens: 25, output_tokens: 25 }),
      row('2026-09-11', { model: 'Beacon', requests: 7, input_tokens: 125, output_tokens: 125, missing_usage: 2 }),
    ] });
    const before = structuredClone(data);
    const result = usageReport(data, '7d');
    expect(result).toMatchObject({ tokens: 500, requests: 13, missing: 3, activeDays: 2, favorite: 'Beacon' });
    expect(result.days.find(day => day.date === '2026-09-10')).toEqual({ date: '2026-09-10', input: 150, output: 50, tokens: 200, requests: 5, missing: 1, covered: true });
    expect(result.models).toEqual([
      { model: 'Atlas', input: 175, output: 75, tokens: 250, requests: 6, missing: 1, share: 0.5 },
      { model: 'Beacon', input: 125, output: 125, tokens: 250, requests: 7, missing: 2, share: 0.5 },
    ]);
    expect(data).toEqual(before);
    expect(result.models.reduce((total, model) => total + model.tokens, 0)).toBe(result.tokens);
    expect(result.models.reduce((total, model) => total + model.requests, 0)).toBe(result.requests);
    expect(result.models.reduce((total, model) => total + model.missing, 0)).toBe(result.missing);
  });

  it('retains tokens and model share with unknown prices and no rate entries', () => {
    const result = usageReport(snapshot({ rows: [
      row('2026-09-11', { model: 'Unpriced', input_tokens: 180, output_tokens: 120, input_rate: null, output_rate: null, amount_tenth_micro_usd: null }),
      row('2026-09-11', { model: 'Priced', input_tokens: 80, output_tokens: 20 }),
    ] }), '7d');
    expect(result.tokens).toBe(400);
    expect(result.models.map(model => [model.model, model.tokens, model.share])).toEqual([['Unpriced', 300, 0.75], ['Priced', 100, 0.25]]);
    expect(result.models.reduce((sum, model) => sum + model.share, 0)).toBeCloseTo(1);
  });

  it('chooses Favorite by greatest recorded requests, not tokens, price or displayed model order', () => {
    const result = usageReport(snapshot({ rows: [
      row('2026-09-11', { model: 'Large-token-model', requests: 1, input_tokens: 900_000 }),
      row('2026-09-10', { model: 'Frequent-model', requests: 5, input_tokens: 0, output_tokens: 0 }),
      row('2026-09-11', { model: 'Frequent-model', price_version: 'another', requests: 6, input_tokens: 0, output_tokens: 0 }),
    ] }), '7d');
    expect(result.models[0].model).toBe('Large-token-model');
    expect(result.favorite).toBe('Frequent-model');
  });

  it('breaks Favorite request ties alphabetically by exact model name, independently of row order', () => {
    const rows = [
      row('2026-09-11', { model: 'Zeta', requests: 4, input_tokens: 9999 }),
      row('2026-09-11', { model: 'Atlas-v2', requests: 4 }),
      row('2026-09-11', { model: 'Atlas-v1', requests: 4, input_tokens: 0 }),
    ];
    expect(usageReport(snapshot({ rows }), '7d').favorite).toBe('Atlas-v1');
    expect(usageReport(snapshot({ rows: [...rows].reverse() }), '7d').favorite).toBe('Atlas-v1');
    expect(usageReport(snapshot({ rows }), '7d').models).toHaveLength(3);
  });

  it('keeps anonymous model usage in totals but does not invent a favorite name', () => {
    const result = usageReport(snapshot({ rows: [row('2026-09-11', { model: '', requests: 8 })] }), '7d');
    expect(result).toMatchObject({ tokens: 15, requests: 8, favorite: null });
    expect(result.models[0]).toMatchObject({ model: '', share: 1 });
  });
});

describe('heatmap levels', () => {
  it.each([[0, 0, 0], [0, 100, 0], [1, 100, 1], [25, 100, 1], [26, 100, 2], [50, 100, 2], [51, 100, 3], [75, 100, 3], [76, 100, 4], [100, 100, 4]])('maps %i / %i recorded tokens to level %i', (tokens, max, level) => {
    expect(heatLevel(tokens, max)).toBe(level);
  });
});
