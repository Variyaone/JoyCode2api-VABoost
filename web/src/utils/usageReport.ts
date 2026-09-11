// Variya usage report: calendar aggregation of the existing token ledger, 2026-09-11.
import type { CostSnapshot } from '../api';
export type UsageRange = 'all' | '30d' | '7d';
const DAY = 86_400_000;
export function calendarTime(day: string): number {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) throw new Error('Invalid calendar date');
  const value = Date.parse(`${day}T00:00:00Z`);
  if (!Number.isFinite(value) || new Date(value).toISOString().slice(0, 10) !== day) throw new Error('Invalid calendar date');
  return value;
}
export const calendarDay = (value: number) => new Date(value).toISOString().slice(0, 10);
export interface UsageDay { date: string; input: number; output: number; tokens: number; requests: number; missing: number; covered: boolean }
export interface UsageModel { model: string; input: number; output: number; tokens: number; requests: number; missing: number; share: number }
export function usageReport(snapshot: CostSnapshot, range: UsageRange) {
  const end = calendarTime(snapshot.today);
  const coverage = snapshot.coverage_start ? calendarTime(snapshot.coverage_start.slice(0, 10)) : null;
  const rowDates = snapshot.rows.map(r => calendarTime(r.day)).filter(d => d <= end);
  // "All" means recorded ledger coverage, never the user's lifetime usage.
  const first = coverage ?? (rowDates.length ? Math.min(...rowDates) : end);
  const start = range === 'all' ? Math.min(first, end) : end - (range === '7d' ? 6 : 29) * DAY;
  const days: UsageDay[] = [];
  const dayMap = new Map<string, UsageDay>();
  for (let t = start; t <= end; t += DAY) {
    const date = calendarDay(t);
    const day = { date, input: 0, output: 0, tokens: 0, requests: 0, missing: 0, covered: t >= first && coverage !== null };
    days.push(day); dayMap.set(date, day);
  }
  const modelMap = new Map<string, UsageModel>();
  for (const row of snapshot.rows) {
    const day = dayMap.get(row.day);
    if (!day) continue;
    day.covered = true;
    day.input += row.input_tokens; day.output += row.output_tokens;
    day.tokens += row.input_tokens + row.output_tokens; day.requests += row.requests; day.missing += row.missing_usage;
    const model = modelMap.get(row.model) ?? { model: row.model, input: 0, output: 0, tokens: 0, requests: 0, missing: 0, share: 0 };
    model.input += row.input_tokens; model.output += row.output_tokens;
    model.tokens += row.input_tokens + row.output_tokens; model.requests += row.requests; model.missing += row.missing_usage;
    modelMap.set(row.model, model);
  }
  const tokens = days.reduce((n, d) => n + d.tokens, 0);
  const models = [...modelMap.values()].sort((a, b) => b.tokens - a.tokens || a.model.localeCompare(b.model));
  models.forEach(m => { m.share = tokens ? m.tokens / tokens : 0; });
  let longest = 0, run = 0;
  for (const d of days) { run = d.covered && d.requests > 0 ? run + 1 : 0; longest = Math.max(longest, run); }
  let last = days.length - 1;
  if (days[last]?.requests === 0 && days[last]?.covered) last--;
  let current = 0;
  for (let i = last; i >= 0 && days[i].covered && days[i].requests > 0; i--) current++;
  const favorite = [...models].filter(m => m.model && m.requests > 0).sort((a, b) => b.requests - a.requests || a.model.localeCompare(b.model))[0]?.model ?? null;
  return { days, models, tokens, requests: days.reduce((n, d) => n + d.requests, 0), missing: days.reduce((n, d) => n + d.missing, 0), activeDays: days.filter(d => d.covered && d.requests > 0).length,
    longest, current, favorite,
    start: calendarDay(start), end: snapshot.today, coverageStart: snapshot.coverage_start || null, timezone: snapshot.timezone,
    hasCoverage: days.some(d => d.covered), partial: days.some(d => !d.covered),
  };
}
export type UsageReport = ReturnType<typeof usageReport>;
export const heatColors = ['#EDF0F5', '#D6E3F6', '#9BBDEB', '#548ACD', '#265A9D'];
export function heatLevel(tokens: number, max: number) {
  if (tokens <= 0 || max <= 0) return 0;
  return Math.min(4, Math.max(1, Math.ceil(tokens / max * 4)));
}
