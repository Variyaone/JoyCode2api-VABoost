// Modified by Variya, 2026-09-11: light charts and restrained data styling.
import { useMemo, useState } from 'react';
import { Card, Col, Row, Segmented, Table, Typography } from 'antd';
import { Area, AreaChart, CartesianGrid, Legend, Line, ComposedChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import type { Stats } from '../api';
import { colors } from '../theme';
import { fmt } from '../utils/dashboard';

export default function DashboardTrends({ stats, at }: { stats: Stats; at: number }) {
  const [view, setView] = useState('chart');
  const rows = useMemo(() => {
    const byHour = new Map((stats.hourly ?? []).map(h => [h.hour, h]));
    return Array.from({ length: 24 }, (_, i) => {
      const d = new Date(at - (23 - i) * 3600000);
      const key = `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}`;
      const h = byHour.get(key);
      return { hour: key, label: `${String(d.getHours()).padStart(2, '0')}:00`, requests: h?.count ?? 0, errors: h?.errors ?? 0, tokens: (h?.input_tokens ?? 0) + (h?.output_tokens ?? 0) };
    });
  }, [stats, at]);
  const tipStyle = { background: colors.surface, border: `1px solid ${colors.border}`, borderRadius: 8, color: colors.text };
  return <section aria-label="24 小时趋势" className="jc-trends">
    <div className="jc-section-toolbar">
      <Typography.Text type="secondary">最近 24 小时 · 请求与 Token</Typography.Text>
      <Segmented aria-label="趋势呈现方式" value={view} onChange={setView} options={[{ label: '图表', value: 'chart' }, { label: '数据表', value: 'table' }]} />
    </div>
    {view === 'table' ? <Table size="small" rowKey="hour" pagination={false} scroll={{ x: 480 }} dataSource={rows} columns={[
      { title: '时间', dataIndex: 'hour' }, { title: '请求数', dataIndex: 'requests', align: 'right' },
      { title: '失败数', dataIndex: 'errors', align: 'right' }, { title: 'Token', dataIndex: 'tokens', align: 'right', render: (n: number) => n.toLocaleString() },
    ]} /> : <Row gutter={[16, 16]}>
      <Col xs={24}><Card size="small" title="24 小时请求趋势">
        <ResponsiveContainer width="100%" height={220}>
          <ComposedChart data={rows} margin={{ top: 8, right: 16, left: 0, bottom: 0 }} accessibilityLayer>
            <CartesianGrid vertical={false} stroke={colors.grid} />
            <XAxis dataKey="label" interval={3} tick={{ fontSize: 12, fill: colors.muted }} stroke={colors.grid} />
            <YAxis tick={{ fontSize: 12, fill: colors.muted }} stroke={colors.grid} allowDecimals={false} width={44} />
            <Tooltip contentStyle={tipStyle} labelStyle={{ color: colors.muted }} itemStyle={{ color: colors.text }}
              labelFormatter={(_, payload) => payload?.[0]?.payload.hour ?? ''}
              formatter={(v, name) => [Number(v).toLocaleString(), name]} />
            <Legend formatter={name => <span style={{ color: colors.muted }}>{name}</span>} />
            <Area type="linear" dataKey="requests" name="请求数" stroke={colors.accent} fill={colors.accent} fillOpacity={0.12} strokeWidth={2} isAnimationActive={false} />
            <Line type="linear" dataKey="errors" name="失败数" stroke={colors.danger} strokeDasharray="4 3" strokeWidth={2} dot={false} activeDot={{ r: 4 }} isAnimationActive={false} />
          </ComposedChart>
        </ResponsiveContainer>
      </Card></Col>
      <Col xs={24}><Card size="small" title="24 小时 Token 消耗趋势">
        <ResponsiveContainer width="100%" height={220}>
          <AreaChart data={rows} margin={{ top: 8, right: 16, left: 0, bottom: 0 }} accessibilityLayer>
            <CartesianGrid vertical={false} stroke={colors.grid} />
            <XAxis dataKey="label" interval={3} tick={{ fontSize: 12, fill: colors.muted }} stroke={colors.grid} />
            <YAxis tick={{ fontSize: 12, fill: colors.muted }} stroke={colors.grid} tickFormatter={fmt} width={52} />
            <Tooltip contentStyle={tipStyle} labelStyle={{ color: colors.muted }} itemStyle={{ color: colors.text }}
              labelFormatter={(_, payload) => payload?.[0]?.payload.hour ?? ''}
              formatter={v => [Number(v).toLocaleString(), 'Token']} />
            <Area type="linear" dataKey="tokens" name="Token" stroke={colors.accent} fill={colors.accent} fillOpacity={0.12} strokeWidth={2} isAnimationActive={false} />
          </AreaChart>
        </ResponsiveContainer>
      </Card></Col>
    </Row>}
  </section>;
}
