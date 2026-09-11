import type { UsageActivity } from '../api';
import { calendarDay, calendarTime } from './usageReport';

export type HourState = 'recorded' | 'zero' | 'partial' | 'unknown' | 'future';
export interface ActivityCell {
  date: string;
  hour: number;
  requests: number | null;
  input: number | null;
  output: number | null;
  tokens: number | null;
  state: HourState;
}

// Hour evidence comes from retained request logs, never divided from daily totals.
export function activityRows(snapshot: UsageActivity): { date: string; cells: ActivityCell[] }[] {
  const now = snapshot.generated_at;
  const today = snapshot.today;
  const currentHour = Number(now.slice(11, 13));
  const byDay = new Map(snapshot.days.map(day => [day.date, day]));
  const rows = [];
  for (let time = calendarTime(snapshot.from); time <= calendarTime(snapshot.through); time += 86_400_000) {
    const date = calendarDay(time);
    const day = byDay.get(date);
    const byHour = new Map((day?.hours ?? []).map(hour => [hour.hour, hour]));
    const cells = Array.from({ length: 24 }, (_, hour): ActivityCell => {
      const value = byHour.get(hour);
      const future = date > today || (date === today && Number.isFinite(currentHour) && hour > currentHour);
      const aligned = day?.coverage === 'matched';
      const measured = value !== undefined;
      const usable = !future && (measured || aligned);
      return { date, hour,
        requests: usable ? value?.requests ?? 0 : null,
        input: usable ? value?.input_tokens ?? 0 : null,
        output: usable ? value?.output_tokens ?? 0 : null,
        tokens: usable ? (value?.input_tokens ?? 0) + (value?.output_tokens ?? 0) : null,
        state: future ? 'future' : measured ? (aligned ? 'recorded' : 'partial') : aligned ? 'zero' : 'unknown',
      };
    });
    rows.push({ date, cells });
  }
  return rows;
}

export function hourLabel(hour: number) {
  return `${String(hour).padStart(2, '0')}:00–${String(hour + 1).padStart(2, '0')}:00`;
}

export function activityCellLabel(cell: ActivityCell) {
  const title = `${cell.date} ${hourLabel(cell.hour)}`;
  if (cell.state === 'future') return `${title}：尚未发生`;
  if (cell.state === 'unknown') return `${title}：小时记录不足，不能按 0 计算`;
  const value = `${cell.requests?.toLocaleString()} 次请求，${cell.tokens?.toLocaleString()} Token（入 ${cell.input?.toLocaleString()} / 出 ${cell.output?.toLocaleString()}）`;
  return `${title}：${value}${cell.state === 'partial' ? '；仅为现存日志，不完整' : ''}`;
}
