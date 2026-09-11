export function fmt(n: number) {
  if (n >= 1_000_000_000) return (n / 1_000_000_000).toFixed(2) + 'B';
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return n.toLocaleString();
}

export function fmtLatency(ms: number) {
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const remainS = s % 60;
  return `${m}m${remainS > 0 ? ` ${remainS}s` : ''}`;
}

export function percentage(count: number, total: number) {
  if (total === 0) return '—';
  if (count === total) return '100%';
  if (count === 0) return '0%';
  const value = count / total * 100;
  if (value < 0.01) return '<0.01%';
  if (value > 99.99) return '>99.99%';
  return `${Number(value.toFixed(2))}%`;
}
