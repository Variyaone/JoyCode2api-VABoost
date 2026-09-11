// Modified by Variya, 2026-09-11: light charts and restrained data styling.
import { Card, Col, Row, Statistic, Empty, Typography, Table, Tag, Divider, Tooltip as AntTooltip } from 'antd';
import { ThunderboltOutlined, CheckCircleOutlined, CloseCircleOutlined, TeamOutlined, ApiOutlined, SwapOutlined, DashboardOutlined, FireOutlined, RiseOutlined, EyeOutlined, SearchOutlined, ExperimentOutlined, HistoryOutlined } from '@ant-design/icons';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { accountDisplayName } from '../api';
import type { Stats, ModelCapability, RequestLog } from '../api';
import { colors } from '../theme';
import { fmt, fmtLatency, percentage } from '../utils/dashboard';

const CHART_COLORS = { primary: colors.accent, secondary: colors.accent, danger: colors.danger, warning: colors.warning, muted: colors.text, grid: colors.border, axis: colors.muted };

export function MoreStats({ stats }: { stats: Stats }) {
  const successRate = percentage(stats.success_count, stats.total_requests);
  const successRateValue = stats.total_requests ? stats.success_count / stats.total_requests * 100 : 0;
  const errorRate = percentage(stats.error_count, stats.total_requests);
  const streamRate = percentage(stats.stream_count, stats.total_requests);
  const totalTokens = stats.total_input_tokens + stats.total_output_tokens;
  const allTimeTokens = (stats.all_time?.total_input_tokens ?? 0) + (stats.all_time?.total_output_tokens ?? 0);
  const avgTokensPerReq = stats.total_requests ? Math.round(totalTokens / stats.total_requests) : 0;
  const avgLatency = Math.round(stats.avg_latency_ms);
  const modelData = (stats.by_model ?? []).map(m => ({name:m.model,value:m.count,pct:stats.total_requests ? Math.round(m.count/stats.total_requests*100):0}));
  const accountData = (stats.by_account ?? []).map(a => ({name:accountDisplayName(a),value:a.count,pct:stats.total_requests ? Math.round(a.count/stats.total_requests*100):0}));
  return <>
      {/* 统计面板：今日 + 累计 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        {/* 请求统计 */}
        <Col xs={24} md={8}>
          <Card size="small" style={{ height: '100%' }} title={<span className="jc-section-title"><ApiOutlined />请求统计</span>}>
            <Row gutter={[8, 12]}>
              <Col span={12}>
                <Statistic title="今日请求" value={stats.total_requests} styles={{ content: { fontSize: 20, color: CHART_COLORS.muted } }} />
              </Col>
              <Col span={12}>
                <Statistic title="累计请求" value={stats.all_time?.total_requests ?? 0} styles={{ content: { fontSize: 20 } }} />
              </Col>
              <Col span={12}>
                <Statistic
                  title="今日成功"
                  value={stats.success_count}
                  prefix={<CheckCircleOutlined />}
                  styles={{ content: { fontSize: 18, color: CHART_COLORS.muted } }}
                />
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>占比 {successRate}</Typography.Text>
              </Col>
              <Col span={12}>
                <Statistic
                  title="今日失败"
                  value={stats.error_count}
                  prefix={<CloseCircleOutlined />}
                  styles={{ content: { fontSize: 18, color: stats.error_count > 0 ? CHART_COLORS.danger : CHART_COLORS.primary } }}
                />
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>占比 {errorRate}</Typography.Text>
              </Col>
              <Col span={24}>
                <Divider style={{ margin: '4px 0 8px' }} />
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Statistic
                    title="流式请求"
                    value={stats.stream_count}
                    styles={{ content: { fontSize: 16 } }}
                    prefix={<SwapOutlined />}
                  />
                  <Tag color="default" style={{ height: 'fit-content', marginTop: 20 }}>{streamRate}</Tag>
                </div>
              </Col>
            </Row>
          </Card>
        </Col>

        {/* Token 消费 */}
        <Col xs={24} md={8}>
          <Card size="small" style={{ height: '100%' }} title={<span className="jc-section-title"><FireOutlined />Token 消费</span>}>
            <Row gutter={[8, 12]}>
              <Col span={12}>
                <Statistic title="今日 Token" value={fmt(totalTokens)} styles={{ content: { fontSize: 20, color: CHART_COLORS.muted } }} />
              </Col>
              <Col span={12}>
                <Statistic title="累计 Token" value={fmt(allTimeTokens)} styles={{ content: { fontSize: 20 } }} />
              </Col>
              <Col span={12}>
                <Statistic title="今日输入" value={fmt(stats.total_input_tokens)} styles={{ content: { fontSize: 16 } }} />
              </Col>
              <Col span={12}>
                <Statistic title="今日输出" value={fmt(stats.total_output_tokens)} styles={{ content: { fontSize: 16 } }} />
              </Col>
              <Col span={24}>
                <Divider style={{ margin: '4px 0 8px' }} />
                <Row gutter={8}>
                  <Col span={12}>
                    <Statistic title="平均每请求" value={avgTokensPerReq.toLocaleString()} suffix="tokens" styles={{ content: { fontSize: 15 } }} />
                  </Col>
                  <Col span={12}>
                    <Statistic
                      title="输入/输出比"
                      value={stats.total_output_tokens > 0 ? (stats.total_input_tokens / stats.total_output_tokens).toFixed(1) : '-'}
                      suffix={stats.total_output_tokens > 0 ? ':1' : ''}
                      styles={{ content: { fontSize: 15 } }}
                    />
                  </Col>
                </Row>
              </Col>
            </Row>
          </Card>
        </Col>

        {/* 响应质量 */}
        <Col xs={24} md={8}>
          <Card size="small" style={{ height: '100%' }} title={<span className="jc-section-title"><DashboardOutlined />响应质量</span>}>
            <Row gutter={[8, 12]}>
              <Col span={12}>
                <Statistic
                  title="平均请求耗时"
                  value={stats.total_requests ? fmtLatency(avgLatency) : "—"}
                  prefix={<ThunderboltOutlined />}
                  styles={{ content: { fontSize: 20, color: CHART_COLORS.muted } }}
                />
              </Col>
              <Col span={12}>
                <Statistic
                  title="成功率"
                  value={successRate}
                  prefix={<CheckCircleOutlined />}
                  styles={{ content: { fontSize: 20, color: successRateValue >= 95 ? CHART_COLORS.primary : successRateValue >= 80 ? CHART_COLORS.warning : CHART_COLORS.danger } }}
                />
              </Col>
              <Col span={24}>
                <Divider style={{ margin: '4px 0 8px' }} />
                <Statistic title="流式占比" value={streamRate} prefix={<SwapOutlined />} styles={{ content: { fontSize: 18 } }} />
              </Col>
              <Col span={12}>
                <Statistic title="配置账号" value={stats.accounts_count} prefix={<TeamOutlined />} styles={{ content: { fontSize: 16 } }} />
              </Col>
              <Col span={12}>
                <Statistic title="使用模型" value={stats.by_model.length} prefix={<RiseOutlined />} styles={{ content: { fontSize: 16 } }} />
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>

      {/* 图表面板 */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        {/* 模型分布 — 横向条形图降序排列 + 值标签 (per skill chart guidance) */}
        <Col xs={24} lg={12}>
          <Card size="small" title={<span className="jc-section-title"><RiseOutlined />模型使用分布</span>}>
            {modelData.length > 0 ? (
              <Row>
                <Col xs={24} md={14}>
                  <ResponsiveContainer width="100%" height={220}>
                    <BarChart data={modelData} layout="vertical" margin={{ left: 10 }}>
                      <CartesianGrid stroke={CHART_COLORS.grid} />
                      <XAxis type="number" tick={{ fontSize: 12, fill: CHART_COLORS.axis }} stroke={CHART_COLORS.grid} />
                      <YAxis dataKey="name" type="category" width={110} tick={{ fontSize: 12, fill: CHART_COLORS.axis }} stroke={CHART_COLORS.grid} />
                      <Tooltip
                        contentStyle={{ background: colors.surface, border: `1px solid ${colors.border}`, borderRadius: 8, fontSize: 12 }}
                        itemStyle={{ color: colors.text }}
                        labelStyle={{ color: colors.muted }}
                        formatter={(v: unknown) => [Number(v).toLocaleString(), '请求数']}
                      />
                      <Bar dataKey="value" name="请求数" fill={CHART_COLORS.primary} radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </Col>
                <Col xs={24} md={10}>
                  <div style={{ padding: '4px 0 0 12px' }}>
                    {modelData.map((m) => (
                      <div key={m.name} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 0', borderBottom: `1px solid ${colors.border}` }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
                          <div style={{ width: 8, height: 8, borderRadius: '50%', background: CHART_COLORS.primary, flexShrink: 0 }} />
                          <Typography.Text style={{ fontSize: 12 }} ellipsis>{m.name}</Typography.Text>
                        </div>
                        <div style={{ display: 'flex', gap: 6, alignItems: 'baseline', flexShrink: 0 }}>
                          <Typography.Text style={{ fontSize: 12, fontWeight: 600 }} className="jc-mono">{m.value.toLocaleString()}</Typography.Text>
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>{m.pct}%</Typography.Text>
                        </div>
                      </div>
                    ))}
                  </div>
                </Col>
              </Row>
            ) : (
              <Empty description="暂无数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Card>
        </Col>

        {/* 账号请求分布 */}
        <Col xs={24} lg={12}>
          <Card size="small" title={<span className="jc-section-title"><TeamOutlined />账号请求分布</span>}>
            {accountData.length > 0 ? (
              <Row>
                <Col xs={24} md={14}>
                  <ResponsiveContainer width="100%" height={220}>
                    <BarChart data={accountData}>
                      <CartesianGrid stroke={CHART_COLORS.grid} />
                      <XAxis dataKey="name" tick={{ fontSize: 12, fill: CHART_COLORS.axis }} stroke={CHART_COLORS.grid} />
                      <YAxis tick={{ fontSize: 12, fill: CHART_COLORS.axis }} stroke={CHART_COLORS.grid} />
                      <Tooltip
                        contentStyle={{ background: colors.surface, border: `1px solid ${colors.border}`, borderRadius: 8, fontSize: 12 }}
                        itemStyle={{ color: colors.text }}
                        labelStyle={{ color: colors.muted }}
                        formatter={(v: unknown) => [Number(v).toLocaleString(), '请求数']}
                      />
                      <Bar dataKey="value" name="请求数" fill={CHART_COLORS.primary} radius={[4, 4, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </Col>
                <Col xs={24} md={10}>
                  <div style={{ padding: '4px 0 0 12px' }}>
                    {accountData.map((a) => (
                      <div key={a.name} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 0', borderBottom: `1px solid ${colors.border}` }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
                          <div style={{ width: 8, height: 8, borderRadius: '50%', background: CHART_COLORS.primary, flexShrink: 0 }} />
                          <Typography.Text style={{ fontSize: 12 }} ellipsis>{a.name}</Typography.Text>
                        </div>
                        <div style={{ display: 'flex', gap: 6, alignItems: 'baseline', flexShrink: 0 }}>
                          <Typography.Text style={{ fontSize: 12, fontWeight: 600 }} className="jc-mono">{a.value.toLocaleString()}</Typography.Text>
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>{a.pct}%</Typography.Text>
                        </div>
                      </div>
                    ))}
                  </div>
                </Col>
              </Row>
            ) : (
              <Empty description="暂无数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Card>
        </Col>
      </Row>

  </>;
}

export function CapabilityRecords({ caps }: { caps: ModelCapability[] }) {
  return <>
      {/* 模型能力记录：历史探测不等于公开 Benchmark */}
      {caps.length > 0 ? (
        <Card
          size="small"
          style={{ marginTop: 16 }}
          title={<span className="jc-section-title"><ExperimentOutlined />通道能力记录（历史探测）</span>}
          extra={<Typography.Text type="secondary" style={{ fontSize: 12 }}>非公开评分；空值表示未验证，成功输入量不是完整能力上限</Typography.Text>}
        >
          <Table
            dataSource={caps}
            rowKey="id"
            size="small"
            pagination={false}
            scroll={{ x: 860 }}
            columns={[
              {
                title: '模型',
                dataIndex: 'id',
                key: 'id',
                width: 170,
                render: (id: string) => <Typography.Text strong style={{ fontSize: 12 }}>{id}</Typography.Text>,
              },
              {
                title: 'API 通道',
                dataIndex: 'api',
                key: 'api',
                width: 100,
                render: (a: string) => (
                  <Tag color="default">{a}</Tag>
                ),
              },
              {
                title: '多模态',
                key: 'vision',
                width: 80,
                render: (_: unknown, r: ModelCapability) => r.vision
                  ? <Tag color="default" icon={<EyeOutlined />}>视觉记录</Tag>
                  : <Typography.Text type="secondary">未确认</Typography.Text>,
              },
              {
                title: '推理',
                key: 'reasoning',
                width: 70,
                render: (_: unknown, r: ModelCapability) => r.reasoning
                  ? <Tag color="default">✓</Tag>
                  : <Typography.Text type="secondary">-</Typography.Text>,
              },
              {
                title: '联网搜索',
                key: 'web_search',
                width: 90,
                render: (_: unknown, r: ModelCapability) => r.web_search
                  ? <Tag color="default" icon={<SearchOutlined />}>内置</Tag>
                  : <Typography.Text type="secondary">-</Typography.Text>,
              },
              {
                title: '已验证输入量',
                key: 'measured_ctx',
                width: 130,
                render: (_: unknown, r: ModelCapability) => {
                  const m = r.measured_ctx;
                  if (!m) return <Typography.Text type="secondary">待验证</Typography.Text>;
                  return (
                    <AntTooltip title={`上游目录标称 ${fmt(r.advertised_ctx)}；历史成功请求输入量 ${m.toLocaleString()} tokens，不代表完整上下文能力上限。`}>
                      <span className="jc-mono" style={{ fontWeight: 600, fontSize: 12 }}>
                        ≥ {fmt(m)}
                      </span>
                    </AntTooltip>
                  );
                },
              },
              {
                title: '备注',
                dataIndex: 'notes',
                key: 'notes',
                render: (n: string) => n
                  ? <Typography.Text type="secondary" style={{ fontSize: 12 }}>{n}</Typography.Text>
                  : null,
              },
            ]}
          />
        </Card>
      ) : <Empty description="暂无历史能力记录" />}

  </>;
}

export function RecentRequests({ recentLogs }: { recentLogs: RequestLog[] }) {
  return (
        <Card
          size="small"
          style={{ marginTop: 16 }}
          title={<span className="jc-section-title"><HistoryOutlined />最近请求明细</span>}
          extra={<Tag>{recentLogs.length} 条</Tag>}
        >
          <Table
            dataSource={recentLogs}
            rowKey="id"
            size="small"
            scroll={{ x: 1060 }}
            locale={{ emptyText: '暂无请求记录' }}
            pagination={{ pageSize: 10, hideOnSinglePage: true, size: 'small' }}
            columns={[
              {
                title: '时间',
                dataIndex: 'created_at',
                key: 'created_at',
                width: 150,
                render: (t: string) => (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>{t}</Typography.Text>
                ),
              },
              {
                title: '模型',
                dataIndex: 'model',
                key: 'model',
                width: 150,
                render: (m: string) => <Tag style={{ fontSize: 12 }}>{m}</Tag>,
              },
              {
                title: '端点',
                dataIndex: 'endpoint',
                key: 'endpoint',
                width: 140,
                render: (e: string) => (
                  <Typography.Text code style={{ fontSize: 12 }}>{e}</Typography.Text>
                ),
              },
              {
                title: '模式',
                dataIndex: 'stream',
                key: 'stream',
                width: 70,
                render: (s: boolean) => s ? <Tag color="default">流式</Tag> : <Tag>非流</Tag>,
              },
              {
                title: '状态',
                dataIndex: 'status_code',
                key: 'status_code',
                width: 70,
                render: (c: number) => (
                  <Tag color={c < 400 ? 'default' : 'error'} className="jc-mono">{c}</Tag>
                ),
              },
              {
                title: '请求耗时',
                dataIndex: 'latency_ms',
                key: 'latency_ms',
                width: 80,
                render: (ms: number) => (
                  <span className="jc-mono" style={{ fontSize: 12, color: ms > 30000 ? colors.danger : ms > 10000 ? colors.warning : undefined }}>
                    {fmtLatency(ms)}
                  </span>
                ),
              },
              {
                title: 'Tokens (入/出)',
                key: 'tokens',
                width: 110,
                render: (_: unknown, r: RequestLog) => (
                  <span className="jc-mono" style={{ fontSize: 12 }}>
                    {r.input_tokens > 0 ? fmt(r.input_tokens) : '-'} / {r.output_tokens > 0 ? fmt(r.output_tokens) : '-'}
                  </span>
                ),
              },
              {
                title: '错误',
                dataIndex: 'error_message',
                key: 'error_message',
                ellipsis: true,
                render: (e: string) => e
                  ? <Typography.Text type="danger" style={{ fontSize: 12 }}>{e}</Typography.Text>
                  : null,
              },
            ]}
          />
        </Card>
  );
}
