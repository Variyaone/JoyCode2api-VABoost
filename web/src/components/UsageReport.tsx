// Consumer-first report: spend, daily usage and model distribution from the recorded ledger.
import { useState } from 'react';
import { Button, Empty, Segmented, Table } from 'antd';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip as ChartTooltip, XAxis, YAxis } from 'recharts';
import type { UsageReport } from '../utils/usageReport';
import type { UsageEconomics } from '../utils/usageEconomics';
import { money } from '../utils/costs';
import type { Currency } from '../utils/costs';
import { fmt } from '../utils/dashboard';
import { colors } from '../theme';
import ModelLogo from './ModelLogo';

export function UsageMetrics({ report, economics, currency }: { report: UsageReport; economics: UsageEconomics; currency: Currency }) {
  const input = report.days.reduce((sum, day) => sum + day.input, 0);
  const output = report.days.reduce((sum, day) => sum + day.output, 0);
  const items = [
    ['参考费用', economics.amount === null ? '暂无法估算' : money(economics.amount, currency), `${economics.total.unpriced} 次未知价格，${report.missing} 次零值或缺用量；非实际账单`],
    ['总 Token', fmt(report.tokens), `输入 ${fmt(input)} / 输出 ${fmt(output)}`],
    ['请求次数', report.requests.toLocaleString(), `所选范围 ${report.activeDays} 个有记录日`],
    ['平均每次 Token', report.requests ? fmt(Math.round(report.tokens / report.requests)) : '暂无请求', '已记录 Token / 全部已记录请求；缺用量会拉低均值'],
  ];
  return <section className="jc-usage-metrics" aria-label="Usage overview">{items.map(([label, value, note]) => <article className="jc-usage-metric" key={label}>
    <h3>{label}</h3><div className="jc-usage-number">{value}</div><p>{note}</p>
  </article>)}</section>;
}

type Metric = 'tokens' | 'amount' | 'requests';
function UsageTick({ x = 0, y = 0, payload, vertical = false, currency }: { x?: number; y?: number; payload?: { value: string | number }; vertical?: boolean; currency?: Currency }) {
  const label = vertical ? `${currency === 'CNY' ? '¥' : currency === 'USD' ? '$' : ''}${fmt(Number(payload?.value ?? 0))}` : String(payload?.value ?? '').slice(5);
  return <text x={x} y={y} dy={vertical ? 4 : 16} dx={vertical ? -6 : 0} textAnchor={vertical ? 'end' : 'middle'} fill={colors.muted} fontSize={12}>{label}</text>;
}

export function DailyUsage({ report, economics, currency }: { report: UsageReport; economics: UsageEconomics; currency: Currency }) {
  const [metric, setMetric] = useState<Metric>('tokens');
  const [view, setView] = useState('chart');
  const labels: Record<Metric, string> = { tokens: 'Token', amount: '参考费用', requests: '请求次数' };
  const data = economics.days.map(day => ({ ...day, plotted: !day.covered ? null : metric === 'amount' ? day.amount === null ? null : day.amount / 10_000_000 * (currency === 'CNY' ? 7.2 : 1) : day[metric] }));
  const max = data.reduce((value, day) => Math.max(value, day.plotted ?? 0), 0);
  const step = 10 ** Math.floor(Math.log10(max || 1));
  const ceiling = max ? Math.ceil(max / step) * step : 1;
  return <section className="jc-usage-section" aria-label="每日消耗趋势">
    <div className="jc-usage-section-head"><h2>每日消耗趋势</h2><div className="jc-daily-controls">
      <Segmented aria-label="每日消耗指标" value={metric} onChange={value => setMetric(value as Metric)} options={[{ label: 'Token', value: 'tokens' }, { label: '参考费用', value: 'amount' }, { label: '请求次数', value: 'requests' }]} />
      <Segmented aria-label="每日 Token 呈现方式" value={view} onChange={setView} options={[{ label: '图表', value: 'chart' }, { label: '每日明细', value: 'table' }]} />
    </div></div>
    <p className="jc-usage-caption">{report.start} 至 {report.end}，按 {report.timezone} 日历日期。{metric === 'amount' ? '费用仅计算有价格且有记录的部分；不是实际扣款。' : '没有记录的日期不等于没有使用；未覆盖日期不当作 0。'}</p>
    {view === 'table' ? <Table rowKey="date" size="small" pagination={{ pageSize: 14, showSizeChanger: false }} scroll={{ x: 640 }} dataSource={economics.days} columns={[
      { title: '日期', dataIndex: 'date' }, { title: '请求次数', align: 'right', render: (_, d) => d.covered ? d.requests.toLocaleString() : '未覆盖' },
      { title: '输入 Token', align: 'right', render: (_, d) => d.covered ? d.input.toLocaleString() : '未覆盖' }, { title: '输出 Token', align: 'right', render: (_, d) => d.covered ? d.output.toLocaleString() : '未覆盖' },
      { title: '参考费用', align: 'right', render: (_, d) => d.amount === null ? '未能估算' : money(d.amount, currency) }, { title: '未知价 / 零值用量', align: 'right', render: (_, d) => `${d.unpriced} / ${d.missing}` },
    ]} /> : <ResponsiveContainer width="100%" height={260}>
      <BarChart data={data} margin={{ top: 16, right: 12, left: 0, bottom: 8 }} accessibilityLayer>
        <CartesianGrid vertical={false} stroke={colors.grid} /><XAxis dataKey="date" minTickGap={32} tick={<UsageTick />} stroke={colors.border} />
        <YAxis domain={[0, ceiling]} width={76} tick={<UsageTick vertical currency={metric === 'amount' ? currency : undefined} />} stroke={colors.border} allowDecimals={metric === 'amount'} />
        <ChartTooltip contentStyle={{ background: colors.surface, color: colors.text, border: `1px solid ${colors.border}` }} itemStyle={{ color: colors.text }} formatter={value => [metric === 'amount' ? `${currency === 'CNY' ? '¥' : '$'}${Number(value).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}` : Number(value).toLocaleString(), labels[metric]]} />
        <Bar dataKey="plotted" name={labels[metric]} fill={colors.accent} maxBarSize={24} radius={[4, 4, 0, 0]} isAnimationActive={false} />
      </BarChart>
    </ResponsiveContainer>}
  </section>;
}

export function ModelUsage({ report, economics, currency, compact = false, onMore }: {
  report: UsageReport; economics: UsageEconomics; currency: Currency; compact?: boolean; onMore?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [rankBy, setRankBy] = useState<'tokens' | 'cost'>('tokens');
  const ordered = [...report.models].sort((a, b) => rankBy === 'cost' ? (economics.models.get(b.model)?.amount ?? -1) - (economics.models.get(a.model)?.amount ?? -1) || b.tokens - a.tokens : b.tokens - a.tokens);
  const modelRows = expanded ? ordered : ordered.slice(0, compact ? 3 : 6);
  return <section className="jc-usage-section" aria-label="模型消耗排行">
    <div className="jc-usage-section-head"><h2>消耗最多的模型</h2><Segmented aria-label="模型消耗排序" value={rankBy} onChange={value => setRankBy(value as 'tokens' | 'cost')} options={[{ label: '按 Token', value: 'tokens' }, { label: '按参考费用', value: 'cost' }]} /></div>
    <p className="jc-usage-caption">与上方时间范围一致。进度条表示 Token 占比；不代表模型能力排名。费用为已知部分，未知价格不视为免费。</p>
    {!modelRows.length ? <Empty description="所选范围暂无模型用量" /> : <ul className="jc-usage-model-list">{modelRows.map(model => {
      const cost = economics.models.get(model.model);
      return <li key={model.model}>
        <div className="jc-usage-model-title"><ModelLogo model={model.model} /><strong>{model.model || '未记录型号'}</strong><span>{(model.share * 100).toFixed(1)}%</span></div>
        <div className="jc-model-meter" aria-hidden="true"><span style={{ width: `${model.share * 100}%` }} /></div>
        <div className="jc-model-usage-detail"><span>输入 {model.input.toLocaleString()}</span><span>输出 {model.output.toLocaleString()}</span><span>{model.requests.toLocaleString()} 次请求</span><span>参考费用 {cost?.amount == null ? '未知' : money(cost.amount, currency)}</span>{model.missing > 0 && <span>{model.missing} 次零值或缺用量</span>}</div>
      </li>;
    })}</ul>}
    {compact ? report.models.length > 3 && <Button type="link" onClick={onMore}>查看全部 {report.models.length} 个模型</Button> : report.models.length > 6 && <Button type="text" onClick={() => setExpanded(!expanded)}>{expanded ? '收起模型' : `展开其余 ${report.models.length - 6} 个模型`}</Button>}
  </section>;
}
