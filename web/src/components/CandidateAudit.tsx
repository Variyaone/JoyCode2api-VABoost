// Frozen, allowlisted local evidence only. This component never probes a model.
import { Alert, Card, Space, Table, Tag, Typography } from 'antd';
import snapshot from '../data/candidateAudit.json';

type AuditModel = (typeof snapshot.models)[number];
type Cell = AuditModel['cells'][number];
const efforts = ['low', 'medium', 'high', 'xhigh', 'max'] as const;
const evidenceById = new Map(snapshot.evidence.map(e => [e.id, e]));
const repairLabels: Record<string, string> = {
  'anonymous-text-deduplication': '匿名文本与 reasoning 快照去重',
  'terminal-text-order': '终态文本按顺序合并；无法修正的流顺序明确报错',
  'tool-index-order': '工具按 index 而非到达顺序输出',
  'native-thinking-budget-cap': 'native thinking 预算与有效 token 上限冲突时在入口拒绝',
  'resolved-model-outbound': '解析后的模型名称用于实际出站请求',
  'responses-small-token-limit': 'Responses 保留正的小额 token 上限',
};
const sourceLabels: Record<string, string> = {
  'frozen-v1': '冻结 v1',
  'snapshot-schema': '展示契约',
  'repair-review': '六项代码补修独立复核',
};

// No enhanced result can upgrade a historical observation or its binary identity.
export function candidateAuditSummary(models: AuditModel[] = snapshot.models) {
  const chatModels = models.filter(m => m.kind === 'chat');
  const cells = chatModels.flatMap<Cell>(m => m.cells);
  return {
    modelCount: models.length,
    totalCells: models.flatMap(m => m.cells).length,
    chatModels: chatModels.length,
    chatCells: cells.length,
    toolExecuted: cells.filter(c => c.status === 'observed' && c.toolExecuted === true).length,
    toolResultReturned: cells.filter(c => c.status === 'observed' && c.toolResultReturned === true).length,
    toolRoundtrips: cells.filter(c => c.status === 'observed' && c.toolExecuted === true && c.toolResultReturned === true).length,
    strictOutput: cells.filter(c => c.status === 'observed' && c.strictOutput === true && c.sdkResultExact === true).length,
    ultraPassed: chatModels.filter(m => m.ultracode.status === 'observed-pass' && m.ultracode.parentBypass === false).length,
  };
}

const enhancedRuns: Record<string, { model: string; test: string }> = {
  e12: { model: 'GLM-5.3', test: 'Bash' },
  e55: { model: 'GPT-6 Astra', test: 'Ultracode' },
};

// A passing flag alone cannot borrow another model's evidence or fixture identity.
export function enhancedPassed(model: AuditModel) {
  const check = model.enhancedVerification;
  const run = check && enhancedRuns[check.evidenceId];
  return model.kind === 'chat' && check?.status === 'passed'
    && run?.model === model.id && run.test === check.test
    && evidenceById.has(check.evidenceId)
    && check.sdkResultExact === true && check.routeVerified === true
    && check.buildIdentity === 'recorded-per-run'
    && check.testFixtureSHA256 === snapshot.candidate.testFixtureSHA256
    && (check.test === 'Bash' || (check.test === 'Ultracode' && check.childIdentityVerified === true));
}

function status(value: boolean | null | undefined, applicable = true, successLabel = '观察通过') {
  if (!applicable) return <Typography.Text type="secondary">不适用</Typography.Text>;
  if (value !== true && value !== false) return <Typography.Text type="secondary">未验证</Typography.Text>;
  return <Typography.Text type={value ? undefined : 'danger'}>{value ? successLabel : '未通过'}</Typography.Text>;
}

function ratio(model: AuditModel, field: 'toolExecuted' | 'toolResultReturned' | 'strictOutput') {
  if (model.kind !== 'chat') return <Typography.Text type="secondary">聊天不适用</Typography.Text>;
  const pass = model.cells.filter(c => c.status === 'observed' && c[field] === true
    && (field !== 'strictOutput' || c.sdkResultExact === true)).length;
  return <span>{pass} / {model.cells.length}</span>;
}

function ultraLabel(model: AuditModel) {
  if (model.kind !== 'chat') return <Typography.Text type="secondary">聊天不适用</Typography.Text>;
  if (model.ultracode.parentBypass === true) return <Typography.Text type="danger">未通过：父代理绕过</Typography.Text>;
  if (model.ultracode.status === 'observed-pass' && model.ultracode.parentBypass === false) return status(true);
  return status(model.ultracode.status === 'failed' ? false : null);
}

function buildLabel(identity: string) {
  return identity === 'not-recorded' ? '该轮构建身份未记录' : '构建身份未验证';
}

function EvidenceMetadata({ id }: { id: string }) {
  const e = evidenceById.get(id);
  if (!e) return <span>证据未记录</span>;
  return <span style={{ overflowWrap: 'anywhere' }}>
    {e.id} · SHA-256 {e.sha256}<br />
    UTC 首个捕获请求 {e.startedAt}；保守结束上界 {e.endedAtUpperBound}
  </span>;
}

function EffortDetails({ model }: { model: AuditModel }) {
  const applicable = model.kind === 'chat';
  return <section aria-label={`${model.id} 档位明细`} style={{ padding: '8px 0' }}>
    <Typography.Paragraph>{model.effortEvidence}。参数接受、回显和透传不证明五档质量或算力逐级提升。</Typography.Paragraph>
    {model.limitations.map(limit => <Typography.Paragraph key={limit}>{limit}</Typography.Paragraph>)}
    {!applicable && <Typography.Paragraph>Base-V3 是代码补全模型，五格聊天结果不适用，不计为失败；不得把其他模型的回退成功计到本模型。</Typography.Paragraph>}
    {model.id === 'MiniMax-M3' && <Typography.Paragraph>medium / high / xhigh / max 工具执行与回传完成，但正文混入推理内容，严格正文及 SDK 精确结果均失败。</Typography.Paragraph>}
    <Table<Cell> size="small" pagination={false} rowKey="effort" dataSource={model.cells} scroll={{ x: 1040 }}
      columns={[
        { title: 'effort', dataIndex: 'effort', width: 80, render: (e: string) => efforts.find(v => v === e) ?? '未知档位' },
        { title: '工具执行', render: (_, c) => status(c.toolExecuted, applicable) },
        { title: '结果回传', render: (_, c) => status(c.toolResultReturned, applicable) },
        { title: '严格正文', render: (_, c) => status(c.strictOutput, applicable) },
        { title: 'SDK 精确结果', render: (_, c) => status(c.sdkResultExact, applicable) },
        { title: '路由证据', width: 180, render: (_, c) => c.routeVerification === 'aggregate-only-not-per-request' ? '仅聚合记录；非逐请求验证' : '未验证' },
        { title: '历史构建', width: 180, render: (_, c) => buildLabel(c.buildIdentity) },
        { title: '证据 ID', dataIndex: 'evidenceId', width: 90 },
      ]} />
    <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
      严格正文要求进程成功、无 SDK 错误、正文与 SDK 最终 result 均精确匹配；这些断言是对保存记录重新检查，
      并非全部模型已完成加强后的逐请求路由与子代理身份链验证。
      历史 Ultracode 采用人工 child transcript + journal 审查，不具备新版逐次执行身份断言。
      {buildLabel(model.ultracode.buildIdentity)}；证据 {model.ultracode.evidenceId}。
    </Typography.Paragraph>
    <details>
      <summary>本模型历史证据元数据</summary>
      <ul>{[...model.cells.map(c => c.evidenceId), model.ultracode.evidenceId].map(id => <li key={id}><EvidenceMetadata id={id} /></li>)}</ul>
    </details>
  </section>;
}

function EnhancedChecks() {
  const checked = snapshot.models.filter(m => m.enhancedVerification !== null);
  return <section aria-label="独立加强验证" style={{ marginBottom: 20 }}>
    <Typography.Title level={5}>GPT / GLM 加强验证 · 与历史矩阵分开</Typography.Title>
    <Typography.Paragraph>只适用于下列两次有构建绑定的具体运行，不是全模型复验，也不将历史 Ultracode 观察升级为同等断言。</Typography.Paragraph>
    {checked.map(model => {
      const check = model.enhancedVerification!;
      return <div key={model.id} aria-label={`${model.id} 加强验证`} style={{ marginBottom: 16 }}>
        <Typography.Text strong>{model.id} · {check.test}</Typography.Text>{' — '}
        {status(enhancedPassed(model), true, '加强检查通过')}
        <Typography.Paragraph style={{ marginTop: 8, marginBottom: 4 }}>
          SDK 精确结果：{status(check.sdkResultExact)}；逐请求路由：{status(check.routeVerified)}；
          子代理身份链：{check.test === 'Bash' ? '不适用（本次是 Bash，不是新的 Ultracode 复验）' : status(check.childIdentityVerified)}。
        </Typography.Paragraph>
        {check.test === 'Ultracode' && <Typography.Paragraph>
          本次核对逐次执行身份、child 结果配对、journal、父 SDK result 与逐请求路由。
          父运行首次触及 180s 截止而超时；保留首次失败，随后以 360s 总截止重试一次，约 181s 通过。
          重试成功不等于首轮成功或稳定性保证。
        </Typography.Paragraph>}
        <Typography.Paragraph type="secondary" style={{ overflowWrap: 'anywhere' }}>
          该次运行绑定测试夹具 SHA-256：{check.testFixtureSHA256}<br /><EvidenceMetadata id={check.evidenceId} />
        </Typography.Paragraph>
      </div>;
    })}
    <Typography.Paragraph type="secondary">其余 11 个目录模型没有冻结的加强验证记录；不得显示为同等验证通过。</Typography.Paragraph>
  </section>;
}

export default function CandidateAudit() {
  const summary = candidateAuditSummary();
  return <Card size="small" className="jc-candidate-audit" style={{ marginTop: 16 }}
    title={<span className="jc-section-title">本机候选兼容性实测（2026-09-11）</span>}>
    <Alert type="warning" showIcon style={{ marginBottom: 16 }}
      title={<Space wrap>
        <Typography.Text strong type="danger">NOT DEPLOYED · 未部署</Typography.Text>
        <Tag>六项代码补修已独立复核</Tag>
        <Tag>GPT / GLM 加强检查通过</Tag>
        <Tag color="warning">NOT RELEASE READY · 未批准发布</Tag>
      </Space>}
      description="候选实测冻结 v1，不是正式部署。六项已确认代码缺陷补修后独立复核通过，结论仅限这六项，不是全库无缺陷保证。GPT 与 GLM 的加强检查已完成，但范围不同，不表示所有模型通过相同断言。正式服务未由此快照证明。" />
    <section aria-label="六项代码补修独立复核">
      <details>
        <summary>六项已确认缺陷：补修后独立复核通过</summary>
        <ul>{snapshot.codeReview.items.map(item => <li key={item.id}>
          {repairLabels[item.id]} — {status(item.verified, true, '独立复核通过')}
        </li>)}</ul>
        <Typography.Paragraph>证据 repair-review；限定这六项修复，不扩展为全库无缺陷保证。</Typography.Paragraph>
      </details>
    </section>
    <Typography.Paragraph type="secondary">
      快照 {snapshot.snapshotId} · 冻结时间（UTC）：<time dateTime={snapshot.frozenAt}>{snapshot.frozenAt}</time><br />
      选定工具 / Ultracode 记录：{snapshot.collection.startedAt} 至 {snapshot.collection.endedAtUpperBound}（保守结束上界）。
      不是整个上游参数探测活动的精确起止；冻结时间不是每项测试的执行时间。
    </Typography.Paragraph>
    <EnhancedChecks />
    <section aria-label="历史兼容性矩阵">
      <Typography.Title level={5}>历史观察 · 构建身份未记录</Typography.Title>
      <Typography.Paragraph>
        <strong>{summary.modelCount} 个目录模型，{summary.totalCells} 格，{summary.chatModels} 个适用聊天。</strong>{' '}
        工具执行 {summary.toolExecuted} / {summary.chatCells} 格；结果回传 {summary.toolResultReturned} / {summary.chatCells} 格；
        工具闭环 {summary.toolRoundtrips} / {summary.chatCells} 格；严格正文 {summary.strictOutput} / {summary.chatCells} 格；
        历史 Ultracode 父子闭环 {summary.ultraPassed} / {summary.chatModels} 个聊天模型。
        Base-V3 的五格聊天结果不适用，不纳入聊天分母。展开模型查看 low / medium / high / xhigh / max 五档证据。
      </Typography.Paragraph>
      <Typography.Paragraph>
        历史 65 格与 13 个 Ultracode 记录跨中间构建，该轮构建身份未记录。
        仅有聚合路由记录，不是逐请求精确路由验证；历史通过不等于全部通过加强后的自动验收。
        部分结果来自重试，历史通过数不构成稳定性保证。参数接受、回显和透传不证明五档质量或算力逐级提升。
      </Typography.Paragraph>
      <Table<AuditModel> size="small" rowKey="id" pagination={false} dataSource={snapshot.models} scroll={{ x: 1200 }}
        expandable={{ expandedRowRender: model => <EffortDetails model={model} />, columnTitle: <span aria-label="展开档位明细">明细</span> }}
        columns={[
          { title: '模型', width: 180, render: (_, m) => <Typography.Text strong>{m.id}</Typography.Text> },
          { title: '工具执行', width: 90, render: (_, m) => ratio(m, 'toolExecuted') },
          { title: '结果回传', width: 90, render: (_, m) => ratio(m, 'toolResultReturned') },
          { title: '严格正文', width: 90, render: (_, m) => ratio(m, 'strictOutput') },
          { title: '历史 Ultracode', width: 160, render: (_, m) => ultraLabel(m) },
          { title: 'effort 证据（非质量评测）', width: 280, dataIndex: 'effortEvidence' },
          { title: '历史构建', width: 180, render: (_, m) => buildLabel(m.ultracode.buildIdentity) },
        ]} />
      <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
        Ultracode 是 xhigh 配合多代理编排，不是第六档，也不等于 max。父代理直接调用工具不能代替子代理成功。
        未记录的加强验证不能计作通过；Base-V3 不能显示为聊天支持或其他模型的回退成功。
      </Typography.Paragraph>
    </section>
    <section aria-label="证据范围与未收录项">
      <Typography.Title level={5}>证据范围与未收录项</Typography.Title>
      <Typography.Paragraph>
        冻结 v1 保留描述性 effort 边界，没有收录旧动态快照的逐档原始探测布尔值、完整重试列表及协议测试计数；
        本页不拼接旧四源文件，也不据此重新计算原始探测结论。GPT 回显、Claude 接受并拒绝非法值、部分 Chat 连非法值也接受，不能合并成“推理已验证”。
        Doubao 的 thinking enabled 要求与高档可能折算仍保留，后续候选成功不能反写原始探测失败。
      </Typography.Paragraph>
      <Typography.Paragraph>
        旧动态快照中的 13 个合成 CLI 场景 / 139 条断言未纳入本冻结源，因此不作为本页当前验证计数；
        合成协议测试始终非逐模型实测，非 Desktop 按钮验收，不与模型矩阵相乘。
        Desktop 输入暂存、按钮和取消未由此进行端到端验收；“已送达”不等于 HTTP 已发出。
        取消不保证任意工具终止或回滚副作用，分叉不自动隔离工作目录。
      </Typography.Paragraph>
      <Typography.Paragraph>
        历史“换会话恢复”案例原始流缺失且同时换了模型，不能把全部历史故障归因于代理或截断，本页不表示旧会话已修复。
        固定日期兼容性实测与公开 Benchmark、真实使用统计分开；All / 30d / 7d 不表示本快照已重新测试。
      </Typography.Paragraph>
    </section>
    <section aria-label="候选身份与来源">
      <Typography.Title level={5}>候选身份与来源</Typography.Title>
      <Typography.Paragraph type="secondary" style={{ overflowWrap: 'anywhere' }}>
        打包候选 SHA-256：{snapshot.candidate.packagedExecutableSHA256}<br />
        打包时间（UTC）：{snapshot.candidate.packagedBuiltAt}<br />
        加强运行测试夹具 SHA-256：{snapshot.candidate.testFixtureSHA256}<br />
        基础提交：{snapshot.candidate.baseCommit}；存在未提交后端差异。
      </Typography.Paragraph>
      <Typography.Paragraph>
        打包哈希仅标识独立候选主程序，不是历史实测程序身份，也不证明打包二进制跑过上述加强检查；
        两次加强运行仅绑定各自记录的测试夹具哈希。二者均不是正式服务身份，不能回填到旧矩阵或旧 Ultracode 记录。
      </Typography.Paragraph>
      <details>
        <summary>冻结来源元数据（仅受控 ID / 哈希 / 日期）</summary>
        <ul>{snapshot.sources.map(source => <li key={source.id} style={{ overflowWrap: 'anywhere' }}>
          {source.id}（{sourceLabels[source.id]}） · SHA-256 {source.sha256}
        </li>)}</ul>
      </details>
      <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0, fontSize: 12 }}>
        本地只读冻结快照，无实时探测；不包含原始路径、日志、请求内容或会话标识。
        productionDeployed=false；releaseReady=false。正式启用需另行确认维护窗口。
      </Typography.Paragraph>
    </section>
  </Card>;
}
