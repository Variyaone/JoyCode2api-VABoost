# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased] - 2026-11-15

### 模型名解析：上游显示标签 → chat id（GLM 等 JD- 系列此前全部不可用）
- 上游 `joycode_modelList` 里每个模型有两个字段：`label`（显示名）和 `chatApiModel`（真正可调用的 id）。`JD-` 开头的条目都是标签，上游只认 id —— 直接把标签发过去会拿到 6001「当前模型不可用或已下线」。JoyCode IDE 在本地做 label → id 映射后再发请求，代理此前是原样转发，因此 `JD-GLM-5.3`、`JD-GLM-5.2`、`JD-Kimi-K3` 等一票模型全部不可用。
- 新增上游 modelList 缓存 `pkg/joycode/catalog.go`：建立 label/modelId → chatApiModel 映射，30 分钟 TTL，后台刷新；请求路径只读缓存，不等待上游调用。
- 在 `prepareBody` / `prepareAnthropicBody`（发往上游前的唯一收口点）翻译 model，Anthropic / OpenAI / Responses 三条协议同时生效。
- 修复默认模型静默复写：`resolveModel` 原本只认硬编码清单，上游刚发布的 id（例如 `GLM-5.3-jcloud`）会被悄悄换成账号默认模型，而响应回显的是请求名，客户端无法察觉自己跑的是另一个模型。现改为「静态清单 + 动态目录」双查。
- 内置清单补上当前在线的 id（`GLM-5.3-jcloud`、`Kimi-K3.1`、`DeepSeek-V4.1-Flash`），作为上游 modelList 拉取失败时的兜底。
- 实测：`JD-GLM-5.3`、`JD-GLM-5.2`、`JD-Kimi-K3`、`GLM-5.3-jcloud` 全部恢复正常，且 `GLM-5.3-jcloud` 不再被替换成 `GLM-5.3`。
- README 补一条 FAQ：6001 的成因（标签 vs id）与处理方式。

### 推理档位被拒时自动降档（GPT-6 Astra / GPT-6.1 Sol）
- `thinking:disabled` 会翻译成 Responses API 的 `reasoning.effort="none"`，但 GPT-6 Astra 与 GPT-6.1 Sol 只接受 `low/medium/high/xhigh/max`，上游以 `Unsupported value: 'none' is not supported with the '...' model` 拒绝整次请求。
- 代理不写死模型清单：上游的拒绝信息里就列出了该模型接受的档位，解析后取最低的一档重试一次。上游增减支持档位时无需改代码，也不会像写死的清单那样悄悄过期。
- 只修复代理自己从 `thinking:disabled` 推导出的 `"none"`；调用方显式指定的其他 effort 被拒时按原样透传，不静默改写调用方参数。
- Anthropic 与 OpenAI 两条协议、流式与非流式四条路径均已覆盖。

### Dashboard 模型目录以上游在售列表为准
- `/api/models` 改为返回上游当前发布的模型，每项带上可提交的 id（上游显示标签）、上游接受的 chat id 与来源标记；上游不再发布的内置模型作为尾部保留，历史名称仍可选。
- 响应附带上游目录的抓取时间与是否已加载，便于判断目录是否新鲜。
- 「运维 → 模型参考」新增「上游模型目录（实时）」表：列出上游当前发布的每个模型、它可发送的名称与上游实际接受的 id，并标注目录抓取时间；未取到实时目录时给出提示。
- 三个「默认模型」下拉框（账号页、账号详情页、设置页）改为读取同一份实时目录，不再各自硬编码一份模型数组；上游新增模型后自动出现在下拉框里。原硬编码清单降级为目录读取前的兜底。
- 修复账号详情页的模型合并：`/api/accounts/{id}/models` 之前原样返回上游字段（`label`/`chatApiModel`），而前端按 `id`/`name` 取值，导致合并出来的选项是空值；后端现统一为 `{id, name, chat_api_model, source}`。

## [Unreleased] - 2026-10-16

### 部署脚本升级（deploy-without-admin.bat）
- 重写为自包含 7 步事务化部署流程：定位新二进制 → 停服 → 备份 → 替换 → 启动 → 健康检查（约 90s）→ 收尾；失败自动回滚（`.bad` 隔离 + `.bak` 恢复 + 重启 + 二次健康检查）。
- 新增参数：`--dry-run`/`-n`（只检查不改动）、`--port N`（默认 34891）、`--help`/`-h`、位置参数指定新二进制路径。
- 自动探测多种新二进制命名（`JoyCode2Api.exe.new`、`JoyCode2Api.new.exe`、`JoyCode2Api-new.exe`、`JoyCode2Api.exe.new.exe`），消除与 `手动部署-openclaw.bat` 的命名不一致。
- 引入 `fc /b` 字节比对：新旧二进制相同时跳过部署，避免无谓停服；备份采用时间戳命名 `JoyCode2Api.exe.bak-<TS>`。
- 修复编码陷阱：原脚本 `chcp 65001` 会导致 cmd.exe 切换代码页后重新解析脚本字节错位，产生 `'-' is not recognized` 等报错；现改为 **GBK 编码 + CRLF 行尾符 + 不使用 chcp**，与 cmd 默认代码页 936 一致。
- 参数解析改用 `ARG1/ARG2` 变量 + `goto` 循环替代 `shift`；dry-run 模式下跳过 `pause` 避免阻塞。

## [Unreleased] - 2026-10-09

### 上游容错（对齐 cc-switch 同类能力，单上游适配版）
- Thinking 签名整流：上游因 thinking 签名不兼容/校验失败拒绝请求时，剥离历史 assistant 消息中的 thinking/redacted_thinking 块并对同一上游重试一次（非流式、流式、原生 Anthropic、Responses 四条路径全覆盖）；剥离前先让重试循环立即跳出，不再对确定性内容错误烧满 3 次原样重试。
- 不支持图片降级：上游明确拒绝图片输入时，将所有 image 块（含 tool_result 内嵌）替换为 `[Unsupported Image]` 文本占位并重试一次，对话不中断。
- 纯文本模型预判：新增已确认纯文本模型注册表（JoyAI-Code-1.5、GLM-5.3、GLM-5.2-jcloud、DeepSeek-V4-Pro、Doubao-Seed-2.0-pro，依据 2026-09-10 实测 vision=false），发送前预剥图片；视觉模型与未知模型不受影响，上游报错兜底仍保留。
- Thinking Budget 错误保持透传不自动修复（维持既有设计哲学：静默改写调用方参数意图不可取）；thinking signature 剥离、图片降级均只处理请求内容，不动参数。
- 块级重写基于原始 JSON，保留块的未知字段（citations、厂商扩展）逐字节原样保留；新增 12 组回归测试覆盖四条路径、正负分类器与字段保真。

## [Preview] - 2026-09-12

- 消费视角首页：撤下未接入的会话/消息占位及重复连续天数，保留四项核心用量/费用指标；每日 Token、参考费用、请求次数趋势和明细表直接置于 Overview，补充模型 Token/费用排序。
- Activity 改为日期 × 小时热力图；新增只读鉴权 `/api/usage-activity`，31 天单页上限、5 秒查询超时、按日及模型核对日志与账本。日志缺失/部分和未来小时区分展示，不均摊每日总量。
- 统一标题、正文、统计与图表字体，中心内容轴对齐；轻视差只在滚动事件时调度单次动画帧，离屏、隐藏、移动端或 reduced-motion 时停用。
- 仅独立端口及脱敏数据副本预览；没有改动正式 exe、正式数据库或推理路由。

## Local deployment — 0.6.1-variya-unified-20260911

- 2026-09-11 23:58（UTC+08:00）完成一次统一切换：已复核后端修复与点阵浅色用量报告一并部署到本地正式服务。
- 独立源码完整 Go 测试、211 项前端测试及 TypeScript 检查通过；正式版本、嵌入资源、GLM 实际文本返回和持续 HTTP 健康检查通过。
- 旧程序及 SQLite 一致性快照与加密配套文件已备份；未改 CC Switch／15721。当前源码尚未因本次部署创建新的 Git 提交或远端发布。
- 模型审计冻结 v1 保留采集时“候选未发布”的原始状态；它不是当前正式部署状态，也不代表本次安装包已逐模型、逐档全量重跑。

## [Unreleased] - 2026-09-11

### Light workspace（已包含在本地统一版本）
- 增加浅色科技点阵、标题区局部蓝青粒子、等宽指标数字与圆点日历；粒子可暂停并记住偏好，隐藏页面和 reduced-motion 停止动画，移动端仅显示四个粒子，不遮挡数据。
- Variya 独立浅色用量报告：Overview / Models 双视图，All / 30d / 7d 统一过滤真实账本，八项概览、日历热力图、每日 Token 柱图与模型占比；不虚构 sessions/messages/peak-hour。
- 原运行明细与费用口径保留为次级工具；模型能力区域新增脱敏候选证据快照，分别显示工具闭环、严格正文、Ultracode、effort 和合成协议测试，不冒称正式部署或五档质量提升。
- 登录、账号、设置、图表、弹窗和代码提示统一浅色；LLM 单色图标适配浅底，彩色品牌保持原色。
- 顶部改为当前维护者链接，底部保留上游项目、维护者、源码版本 0.6.1 和基线提交 176ca9d 的致谢；保留全部许可证。
- 沿用已验证的刷新、历史校验、费用计算和评测排序，不改后端链路或正式 daemon。

### Dashboard
- 数据概览拆分为运行概览、费用明细、模型参考；保留现有统计、费用口径、评测来源与排序，优先展示运行趋势和最近请求。
- 默认 30 秒自动刷新，支持暂停、手动刷新、后台暂停及返回前台补刷；资源独立更新时间，失败保留旧数据并提示过期。
- 代理 HTTP 可达与账号历史校验分开显示，不再把配置账号数称为在线；校验失败不直接判定凭据过期。
- GLM 使用 Z.ai 图标，单色品牌标识改为适配深色背景的浅色版本。
- 新增 111 项前端测试及账号状态接口回归；修复无请求时成功率和整数取整掩盖失败的问题。

### Fixed
- 后端兼容性修复（已包含在本地统一版本）：保留并转换 `output_config.effort`，按 GPT/Claude/Chat 通道传递推理控制；MiniMax 保留 adaptive，Doubao 补齐所需 thinking 开关。不保证所有上游均实现五个独立推理档。
- GPT/Chat 工具输出延迟到成功终态并校验完整 JSON 对象及非空工具名；断流、缺失终态和失败不再伪装正常完成，max_tokens 不执行半成品工具；支持 Responses 快照补全与去重，修复末尾多余空文本块导致 SDK/Workflow result 为空。
- 上下文截断按工具 ID 保留跨中途 system 消息的调用/结果配对；对代码补全专用 JoyCode-Base-V3 明确拒绝聊天请求，不静默替换为其他模型。
- 修复 GPT Responses 转换丢失执行中追加文字：Claude Code 2.1.260 会将 queued_command 渲染为会话中的 system 消息，旧转换仅保留第一条 system/developer，导致后续补充要求被静默丢弃。现在仅提取开头的指令为 instructions，后续指令保留角色、内容和顺序。
- 新增多条指令、工具结果后的追加消息，以及 Anthropic 流式/非流式到 Responses 实际发送内容的回归测试。

### Added
- 公开模型单价及今日/累计/逐日费用估算（USD），明确未知价格和缺失用量；版本化定点单价、幂等历史回填、独立汇总账本不随日志清理丢失。
- `/api/costs` JWT 接口和费用面板；统一登录、初始化、主布局和 README 的学习研究、信息安全及责任限制声明。
- GPT-6 Astra、Claude-Opus-5 名称与 Claude `-hq` 映射支持、下拉选项同步（基础文本验证）。
- Dashboard「公开模型评测」

### Changed（20260911 v2）
- 金额显示两位小数并右对齐；人民币/美元按 1 USD ≈ 7.2 CNY 固定参考汇率切换（非实时汇率，仅用于估算展示）。
- 按模型累计表取消分页；模型行添加品牌色块标识。
- jcloud 与国产模型按各官方站价格计价：Kimi-K3（jcloud 同）按 Kimi 官网、GLM 按智谱官网、DeepSeek-V4-Pro 按 DeepSeek 官网 peak 价、Doubao-Seed-2.0-pro 按火山方舟 (32,128] 阶梯价；均标注为参考价。
- 评分卡改为多维对比（AA 指数 + BenchLM 综合分），仅展示有可核实成绩的模型，移除实时刷新按钮与 Arena 空源。：AA Intelligence Index v4.3 分数、多推理档位、模型筛选、同指标排序、来源链接及采集日期。
- `/api/model-benchmarks`：随程序嵌入的 JSON 公开评分快照，沿用 Dashboard JWT 鉴权，不自动访问公网。
- SWE-bench 官方榜单与 Arena 来源状态说明；缺失/访问受限记录不填分数。jcloud 基础模型参考、DeepSeek 日期版本与名称对应记录分开处理。

### Corrected
- 公开最高档分数不是 JoyCode 当前部署的成绩，不与本地能力测试混排。
- 撤回旧文档中全系列“均已实测 1M”、GPT 精确 910k、JoyAI 精确 180k 等外推说法；能力面板改为成功输入量下界及待验证标注。
- 旧条目所称“全链路工具闭环”“SSE 一定返回”“内置搜索已接通”等不是本次验证结论；需以具体协议测试为准。图片失败、访问受限不能推导为服务绝对不支持该能力。

## [Unreleased] - 2026-09-10

### Added
- **GPT-5.6 Sol 全链路支持（Responses API 通道）**：
  - GPT 系模型只接受 OpenAI Responses API，旧 Chat Completions 通道对其返回错误。新增 `responses_completions` 网关端点与双向协议翻译层（`pkg/joycode/responses_translate.go`、`pkg/openai/responses.go`）。
  - 四条路径（Anthropic 流式/非流式、OpenAI 流式/非流式）全部打通；Claude Code 的工具调用循环（function_call 生成 → 参数传递 → tool_result 回传）验证通过。
  - 按模型名前缀自动分流（`IsResponsesAPIModel`），调用方无感知。
  - 支持 Responses 内置 `web_search` 工具（`tools: [{"type":"web_search"}]`）。
- **Dashboard 模型能力矩阵**：新增 `/api/model-capabilities` 端点与前端面板，展示每个模型实测的 API 通道、多模态、推理、联网搜索与真实上下文上限（隐藏码字召回法探测）。
- **Dashboard 请求明细**：新增 `/api/recent-logs` 端点与前端表格，展示最近请求的模型、端点、状态码、延迟、Token 用量与错误信息（此前只有错误列表，正常请求不可见）。

### Fixed
- **修复 Claude 系列经代理无输出（EOF）**：上游原生 Anthropic 端点要求内部模型名带 `-hq` 后缀（如 `Claude-Opus-4.8-hq`），此前代理发送裸名导致 6002 错误。现已自动映射全部 Claude 模型。
- **修复 GPT 流式空回复**：上游 `stream:false` 时也返回 SSE，非流式路径改为流式聚合；reasoning 模型小 `max_tokens`（<4096）会被内部推理消耗光导致正文为空（`incomplete_details.reason=max_output_tokens`），现对小于 4096 的值不传该参数。
- **修复工具 schema 丢失**：`input_schema`（`json.RawMessage`）类型断言失败导致工具参数定义序列化为空 `{}`，模型收不到参数结构、调用参数为空。现兼容 RawMessage / map / string 三种形态。

### Changed
- **真实上下文上限**：实测 GLM-5.3 / Kimi-K3 / DeepSeek-V4-Pro / Claude 全系均可接受约 100 万 token（官方标称 200k 为保守值）；Claude 后端为 Bedrock，硬上限 1,000,000 token。上游请求体硬上限 5MB。

## [Unreleased] - 2026-08-31

### Added
- **全新模型矩阵支持**：
  - **Claude 系列**：Claude-Opus-4.8、Claude-Sonnet-4.6、Claude-Opus-4.6（及原有 4.7）
  - **智谱 GLM 系列**：GLM-5.3、GLM-5.2-jcloud（JDCloud 数据出域）
  - **月之暗面 Kimi 系列**：Kimi-K3、Kimi-K3-jcloud（JDCloud 数据出域）
  - **深度求索 DeepSeek**：DeepSeek-V4-Pro
  - **MiniMax 系列**：MiniMax-M3（及原有 M2.7）
  - **OpenAI 系列**：GPT-5.6 Sol
- **Windows 便捷启动脚本**：
  - 新增 `启动JoyCode2Api.bat`，双击即可一键以 HTTP 模式（`--tls=false --skip-validation`）启动本地代理服务。

### Fixed
- **修复 Claude 系列模型无输出问题**：
  - `enable_claude` 开关改为默认开启（opt-out 语义）。此前默认关闭时，Claude 请求会降级走旧 OpenAI 路径，而 Claude 原生模型会拒绝该路径，导致客户端无任何输出。
  - 现在 Claude 请求默认走原生 Anthropic 端点 `/api/saas/anthropic/v1/messages`；如需强制回退旧路径，可显式将 `enable_claude` 设为 `"false"`。

### Changed
- **模型路由与能力匹配**：
  - 优化 Anthropic 协议模型解析，支持所有以 `Claude` 开头的原生模型透传与合理回退。
  - 扩充模型能力配置（Vision、Reasoning 标识及上下文上限），将新系列推理模型纳入 Reasoning 白名单。
- **前端 Web 控制台同步**：
  - 同步更新 Dashboard 账号管理、账号详情与系统设置中的模型下拉选择与 Claude 模型判定逻辑。