// The models the upstream publishes right now.
//
// The upstream list changes without a release, so this is the place to look
// for a model the proxy can serve: every entry shows the display label a
// client may send and the id the upstream actually accepts for it.
import { useMemo, useState } from 'react';
import { Alert, Card, Empty, Input, Space, Table, Tag, Typography } from 'antd';
import { CloudServerOutlined } from '@ant-design/icons';
import ModelLogo from './ModelLogo';
import ResourceStatus from './ResourceStatus';
import type { ResourceState } from './ResourceStatus';
import type { ModelCatalog } from '../api';

export default function UpstreamModels({ resource }: {
  resource: ResourceState & { data: ModelCatalog | null; initialLoading: boolean };
}) {
  const { data, initialLoading: loading, error } = resource;
  const [query, setQuery] = useState('');

  const models = useMemo(() => data?.models ?? [], [data]);
  const upstreamCount = useMemo(() => models.filter(m => m.source === 'upstream').length, [models]);
  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return models;
    return models.filter(m => m.id.toLowerCase().includes(q) || (m.chat_api_model ?? '').toLowerCase().includes(q));
  }, [models, query]);

  return (
    <Card size="small" className="jc-upstream-models" style={{ marginTop: 16 }}
      title={<span className="jc-section-title"><CloudServerOutlined />上游模型目录（实时）</span>}>
      <ResourceStatus label="上游目录" resource={resource} />
      {error && !data && <Alert type="warning" showIcon title="上游目录读取失败" description="请检查本地代理连接后重试。" style={{ marginBottom: 12 }} />}
      {data && !data.upstream_loaded && (
        <Alert type="info" showIcon title="尚未取到上游实时目录" style={{ marginBottom: 12 }}
          description="下列为内置清单。代理收到请求后会再向上游拉取一次目录，届时新模型会自动出现。" />
      )}
      <Space wrap style={{ marginBottom: 12 }}>
        <Input.Search aria-label="筛选上游模型" placeholder="搜索模型名称或上游 id" allowClear value={query}
          onChange={e => setQuery(e.target.value)} style={{ width: 240 }} />
        <Tag color="default">{upstreamCount} 个上游在列</Tag>
        {data?.upstream_at && <Typography.Text type="secondary">目录读取时间：{new Date(data.upstream_at).toLocaleString('zh-CN')}</Typography.Text>}
      </Space>
      {loading && !data ? <Empty description="读取中" /> : (
        <Table rowKey="id" size="small" pagination={false} scroll={{ x: 520 }} dataSource={rows}
          locale={{ emptyText: '没有匹配的模型' }}
          columns={[
            { title: '模型（可发送的名称）', dataIndex: 'id', width: 230, render: (id: string) => (
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                <ModelLogo model={id} />
                <Typography.Text strong>{id}</Typography.Text>
              </span>
            ) },
            { title: '上游实际使用的 id', dataIndex: 'chat_api_model', width: 230, render: (v: string, m) => (
              v && v !== m.id
                ? <Typography.Text code>{v}</Typography.Text>
                : <Typography.Text type="secondary">与名称相同</Typography.Text>
            ) },
            { title: '来源', dataIndex: 'source', width: 100, render: (source: string) => (
              source === 'upstream'
                ? <Tag color="blue">上游在列</Tag>
                : <Tag>内置历史</Tag>
            ) },
          ]} />
      )}
      <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0, fontSize: 12 }}>
        上游只接受「上游实际使用的 id」列里的名称；代理会自动翻译上表左列的名称，两种写法都能用。「内置历史」是上游不再发布的历史名称，不保证仍可用。
      </Typography.Paragraph>
    </Card>
  );
}
