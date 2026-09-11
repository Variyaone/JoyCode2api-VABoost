import type { CostSnapshot } from '../api';
import { summary } from './costs';
import type { UsageReport } from './usageReport';

// Pricing remains the existing recorded ledger math; no provider billing is inferred.
export function usageEconomics(report: UsageReport, snapshot: CostSnapshot) {
  const rows = snapshot.rows.filter(row => row.day >= report.start && row.day <= report.end);
  const grouped = new Map<string, typeof rows>();
  const models = new Map<string, typeof rows>();
  for (const row of rows) {
    const dayRows = grouped.get(row.day) ?? [];
    dayRows.push(row);
    grouped.set(row.day, dayRows);
    const modelRows = models.get(row.model) ?? [];
    modelRows.push(row);
    models.set(row.model, modelRows);
  }
  const total = summary(rows);
  const amount = total.known || total.requests === 0 ? total.amount : null;
  return {
    total, amount,
    days: report.days.map(day => {
      const cost = summary(grouped.get(day.date) ?? []);
      return { ...day, amount: day.covered && (cost.known || cost.requests === 0) ? cost.amount : null, unpriced: cost.unpriced };
    }),
    models: new Map([...models].map(([model, entries]) => {
      const cost = summary(entries);
      return [model, { ...cost, amount: cost.known || cost.requests === 0 ? cost.amount : null }];
    })),
  };
}
export type UsageEconomics = ReturnType<typeof usageEconomics>;
