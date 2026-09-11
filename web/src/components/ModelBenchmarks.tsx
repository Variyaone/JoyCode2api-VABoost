// Modified by Variya, 2026-09-11: shared model identity and light presentation.
import ModelLogo from './ModelLogo';
import { useMemo, useState } from 'react';
import { Alert, Card, Empty, Input, Segmented, Space, Spin, Table, Tag, Typography } from 'antd';
import { TrophyOutlined } from '@ant-design/icons';
import ResourceStatus from './ResourceStatus';
import type { ResourceState } from './ResourceStatus';
import type { BenchmarkModel, BenchmarkResult, BenchmarkSnapshot } from '../api';

const mappingLabels: Record<BenchmarkModel['mapping'], string> = {
  name_match: '名称对应 · 配置未核对',
  deployment_reference: '基础模型参考',
  version_ambiguous: '版本待确认',
  unverified: '暂无可核实数据',
};

function pick(model: BenchmarkModel, sourceId: string): BenchmarkResult | undefined {
  if (model.mapping !== 'name_match') return undefined;
  return model.results
    .filter(r => r.source_id === sourceId && r.score !== null && !r.footnote)
    .sort((a, b) => (b.score ?? 0) - (a.score ?? 0))[0];
}

type Mode = 'aa' | 'multi';

export default function ModelBenchmarks({ resource }: {
  resource: ResourceState & { data: BenchmarkSnapshot | null; initialLoading: boolean };
}) {
  const { data: snapshot, initialLoading: loading, error } = resource;
  const [mode, setMode] = useState<Mode>('multi');
  const [query, setQuery] = useState('');
  const [ascending, setAscending] = useState(false);

  const aaSource = snapshot?.sources.find(s => s.id === 'aa');
  const benchlmSource = snapshot?.sources.find(s => s.id === 'benchlm');

  // Multi-dimension view: only models with at least one verifiable score.
  const multiRows = useMemo(() => (snapshot?.models ?? [])
    .filter(m => m.results.some(r => r.score !== null))
    .map(m => {
      const aa = m.results.find(r => r.source_id === 'aa' && r.score !== null && !r.footnote);
      const bl = m.results.find(r => r.source_id === 'benchlm' && r.score !== null);
      const variants = m.results.filter(r => r.score !== null);
      return { ...m, aa: aa?.score ?? null, bl: bl?.score ?? null, variants, link: bl?.url ?? aa?.url ?? '' };
    })
    .filter(m => m.aa !== null || m.bl !== null)
    .sort((a, b) => (ascending ? (a.bl ?? a.aa ?? 0) - (b.bl ?? b.aa ?? 0) : (b.bl ?? b.aa ?? 0) - (a.bl ?? a.aa ?? 0))),
    [snapshot, ascending]);

  const searched = multiRows.filter(m => m.id.toLowerCase().includes(query.toLowerCase()));

  const aaRows = useMemo(() => (snapshot?.models ?? [])
    .filter(m => m.id.toLowerCase().includes(query.toLowerCase()))
    .map(m => ({ ...m, best: pick(m, 'aa') }))
    .sort((a, b) => {
      const av = a.best?.score, bv = b.best?.score;
      if (av == null) return bv == null ? a.id.localeCompare(b.id) : 1;
      if (bv == null) return -1;
      return (ascending ? av - bv : bv - av) || a.id.localeCompare(b.id);
    }), [snapshot, query, ascending]);

  const scored = snapshot?.models.filter(m => pick(m, 'aa')).length ?? 0;

  return (
    <Card size="small" className="jc-benchmarks" style={{ marginTop: 16 }}
      title={<span className="jc-section-title"><TrophyOutlined />公开模型评测</span>}
      extra={<Segmented value={mode} onChange={v => setMode(v as Mode)} options={[{ label: '多维对比', value: 'multi' }, { label: 'AA 指数明细', value: 'aa' }]} />}>
      <Spin spinning={loading}>
        <ResourceStatus label="公开评测" resource={resource} snapshot />
        {snapshot ? <>
          <Alert type="info" showIcon title="公开评测参考 ≠ JoyCode 当前表现" description={snapshot.notice} style={{ marginBottom: 16 }} />
          <Space wrap style={{ marginBottom: 12 }}>
            <Input.Search aria-label="筛选评测模型" placeholder="搜索模型名称" allowClear value={query}
              onChange={e => setQuery(e.target.value)} style={{ width: 220 }} />
            <Segmented aria-label="分数排序" value={ascending ? 'asc' : 'desc'}
              onChange={v => setAscending(v === 'asc')}
              options={[{ value: 'desc', label: '从高到低' }, { value: 'asc', label: '从低到高' }]} />
            <Typography.Text type="secondary">采集：{snapshot.collected_at} · 本地快照，非实时</Typography.Text>
          </Space>
          {mode === 'multi' ? (
            <Table rowKey="id" size="small" pagination={false} scroll={{ x: 760 }} dataSource={searched}
              locale={{ emptyText: '没有匹配的模型' }}
              columns={[
                { title: '模型', dataIndex: 'id', width: 200, render: (id: string) => <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  <ModelLogo model={id} />
                  <Typography.Text strong>{id}</Typography.Text></span> },
                { title: 'AA 智能指数 v4.3', align: 'right', width: 150, render: (_, m) => m.aa !== null
                  ? <Typography.Text strong style={{ fontSize: 16, color: '#252A34' }}>{m.aa}</Typography.Text>
                  : <Typography.Text type="secondary">—</Typography.Text> },
                { title: 'BenchLM 综合', align: 'right', width: 130, render: (_, m) => m.bl !== null
                  ? <Typography.Text strong style={{ fontSize: 16, color: '#252A34' }}>{m.bl.toFixed(1)}</Typography.Text>
                  : <Typography.Text type="secondary">—</Typography.Text> },
                { title: '对应关系', width: 175, render: (_, m) => <Tag color={'default'}>{mappingLabels[m.mapping]}</Tag> },
                { title: '可核实维度 / 证据', render: (_, m) => <>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>{m.variants.length} 条已核实记录</Typography.Text>
                  {m.link && <div><a href={m.link} target="_blank" rel="noopener noreferrer">来源详情 ↗</a></div>}
                  <div><Typography.Text type="secondary" style={{ fontSize: 12 }}>{m.note}</Typography.Text></div>
                </> },
              ]}
              expandable={{
                rowExpandable: m => m.variants.length > 0,
                expandedRowRender: m => <div style={{ padding: 8 }}>
                  <Table size="small" pagination={false} rowKey={r => `${r.source_id}-${r.public_model}-${r.variant}`}
                    dataSource={m.variants} columns={[
                      { title: '来源', dataIndex: 'source_id', render: (v: string) => v === 'aa' ? 'Artificial Analysis' : 'BenchLM' },
                      { title: '评测型号', dataIndex: 'public_model' },
                      { title: '档位', dataIndex: 'variant' },
                      { title: '分数', dataIndex: 'score', render: (v: number | null) => v ?? '—' },
                      { title: '说明', dataIndex: 'footnote', render: (v: string) => v || '—' },
                      { title: '证据', dataIndex: 'url', render: (url: string) => <a href={url} target="_blank" rel="noopener noreferrer">原站 ↗</a> },
                    ]} />
                </div>,
              }} />
          ) : (
            <>
              {aaSource && <div style={{ marginBottom: 12 }}>
                <Typography.Text strong>{aaSource.metric} · {aaSource.version}</Typography.Text>{' '}
                <Tag>{aaSource.status}</Tag><Tag>{scored} / {snapshot.models.length} 个名称对应模型有评分</Tag>
                <div style={{ margin: '8px 0', lineHeight: 1.7 }}><Typography.Text type="secondary">{aaSource.description}</Typography.Text></div>
                <Space wrap>
                  <Typography.Link href={aaSource.url} target="_blank" rel="noopener noreferrer">官方榜单 ↗</Typography.Link>
                  <Typography.Link href={aaSource.methodology_url} target="_blank" rel="noopener noreferrer">评测方法 ↗</Typography.Link>
                </Space>
              </div>}
              <Table rowKey="id" size="small" pagination={false} scroll={{ x: 860 }} dataSource={aaRows}
                locale={{ emptyText: '没有匹配的模型' }}
                expandable={{
                  rowExpandable: m => m.results.some(r => r.source_id === 'aa'),
                  expandedRowRender: m => <div style={{ padding: 8 }}>
                    <Typography.Paragraph>{m.note}</Typography.Paragraph>
                    <Table size="small" pagination={false} rowKey={r => `${r.public_model}-${r.variant}`}
                      dataSource={m.results.filter(r => r.source_id === 'aa')} columns={[
                        { title: '公开评测型号', dataIndex: 'public_model' },
                        { title: '评测档位', dataIndex: 'variant' },
                        { title: '指数', dataIndex: 'score', render: (v: number | null) => v ?? '—' },
                        { title: '脚注', dataIndex: 'footnote', render: (v: string) => v || '—' },
                        { title: '发布日期', dataIndex: 'published_at', render: (v: string | null) => v ?? '来源未提供确切日期' },
                        { title: '证据', dataIndex: 'url', render: (url: string) => <a href={url} target="_blank" rel="noopener noreferrer">原站详情 ↗</a> },
                      ]} />
                  </div>,
                }} columns={[
                  { title: 'JoyCode 模型', dataIndex: 'id', width: 180, render: (id: string) => <Typography.Text strong>{id}</Typography.Text> },
                  { title: '公开最高档 · 指数', width: 155, render: (_, m) => m.best
                    ? <Typography.Text strong style={{ fontSize: 21, color: '#252A34' }}>{m.best.score}</Typography.Text>
                    : <Typography.Text type="secondary">—</Typography.Text> },
                  { title: '公开评测档位', width: 130, render: (_, m) => m.best ? <Tag>{m.best.variant}</Tag> : '—' },
                  { title: '对应关系', width: 180, render: (_, m) => <Tag color={'default'}>{mappingLabels[m.mapping]}</Tag> },
                  { title: '说明 / 来源', render: (_, m) => <>
                    <Typography.Text type="secondary">{m.note}</Typography.Text>
                    {m.best && <div><a href={m.best.url} target="_blank" rel="noopener noreferrer">{m.best.public_model} · 原站详情 ↗</a></div>}
                    {!m.best && m.results.some(r => r.source_id === 'aa') && <div><Typography.Text>展开查看参考记录（不参加主排名）</Typography.Text></div>}
                  </> },
                ]} />
            </>
          )}
          <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0, fontSize: 12 }}>
            分数越高表示该指标表现越好，不是百分制正确率。多维对比只列出有可核实评分的模型；jcloud 部署和日期不明版本不进入主排名；缺失数据不计为 0。
            {benchlmSource ? ` BenchLM 快照：${benchlmSource.version}，ESTIMATED 表示有限直接证据。` : ''}
          </Typography.Paragraph>
        </> : !loading && !error && <Empty description="暂无公开评测快照" />}
      </Spin>
    </Card>
  );
}
