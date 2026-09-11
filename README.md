<div align="center">

# joyCode2api-VABoost

**JoyCode → Anthropic / OpenAI 协议翻译器 · VABoost 增强版**

让 Claude Code、Cursor、Codex 直接用上 JoyCode 背后的模型

[GitHub](https://github.com/variyaone/JoyCode2api-VABoost) · [Gitee（国内镜像）](https://gitee.com/variyaone/JoyCode2api-VABoost)

`JoyAI-Code-1.5` · `Claude-Opus-5` · `Claude-Opus-4.8` · `GLM-5.3` · `GLM-5.2-jcloud` · `Kimi-K3` · `Kimi-K3-jcloud` · `DeepSeek-V4-Pro` · `MiniMax-M3` · `Doubao-Seed-2.0-pro` · `GPT-6 Astra` · `GPT-5.6 Sol`

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react)](https://react.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue?style=flat)](./LICENSE)

[快速开始](#快速开始) · [VABoost 增强了什么](#vaboost-增强了什么) · [部署](#部署) · [API 参考](#api-参考) · [FAQ](#faq)

</div>

---

## 这是什么

JoyCode（京东 AI 编程助手）背后挂了 GLM、Kimi、MiniMax、Doubao、Claude-Opus 等模型，但它的 API 是私有协议，主流编程工具接不上。本项目在中间做协议翻译，对外同时暴露两套标准协议：

```
Claude Code ─┐
Cursor    ───┼──→  joyCode2api-VABoost  ──→  JoyCode API (jd.com)
Codex     ───┘    (协议翻译层)
```

- **Anthropic Messages API** (`/v1/messages`) — Claude Code 走这个
- **OpenAI Chat Completions API** (`/v1/chat/completions`) — Cursor / Codex 走这个

工具调用（tool use）、流式输出（SSE）、上下文截断全部完整翻译，使用体验和原生 API 一致。

> ⚠️ **使用声明与责任限制**：本项目仅供大语言模型（LLM）的学习、技术研究及经授权的测试使用，非 JoyCode 官方产品。使用者应遵守适用法律法规、服务提供方条款及所在组织的信息安全、保密和数据处理规定。未经合法授权及必要审批，不得上传、传输或披露公司机密、个人信息、访问凭据及其他敏感数据；严禁用于违法违规活动或侵犯第三方权益。软件按“现状”提供，不对可用性、准确性或特定用途适用性作出保证。在适用法律允许的范围内，使用者应对其使用行为及后果承担责任，作者及贡献者不对因使用或无法使用本项目而产生的损失承担责任；本声明不排除法律规定不得排除或限制的责任。

## VABoost 增强了什么

VABoost 是 [vibe-coding-labs / JoyCode2Api](https://github.com/vibe-coding-labs/JoyCode2Api)（上游 0.6.1，基线 [176ca9d](https://github.com/vibe-coding-labs/JoyCode2Api/commit/176ca9d4d7f98e46a5d979be4cd61b4bc0470b0f)）的独立增强分支，重心在**用量透明与消费视角监控**：

| 增强 | 说明 |
|------|------|
| **消费视角用量报告** | 参考费用、总 Token、请求次数、平均每次 Token 四项核心指标；每日 Token / 参考费用 / 请求趋势图与明细表直接放在首页 |
| **模型消耗排行** | 按模型统计 Token 占比与参考费用，支持 Token / 费用两种排序 |
| **日期 × 小时使用热力图** | 每行一天、每列一小时，看清一天中最常使用的时段；小时数据与每日账本逐日核对，缺失记录不填零、不把日总量均摊到小时 |
| **诚实的数据口径** | 未接入的指标不虚构（无 sessions / messages / peak-hour 占位）；未知价格不当作免费；未覆盖日期不当作用量为零 |
| **双平台仓库徽章** | Dashboard 顶栏 GitHub / Gitee 星标徽章，点击直达一键 Star |
| **浅色工作台** | 统一字体与居中排版；粒子与轻视差仅在可见且允许动态时运行，移动端自动关闭 |
| **推理链路修复** | `output_config.effort` 保留并转换，按 GPT/Claude/Chat 通道传递推理控制；MiniMax 保留 adaptive，Doubao 补齐 thinking 开关 |
| **模型图标修正** | GLM 使用 Z.ai 图标，单色品牌标识适配浅色背景 |

上游原有能力（多账号管理、扫码 / OAuth 登录、凭据保活、智能上下文截断、模型能力矩阵、单文件部署、Docker / 系统服务）全部保留。

## 快速开始

> **TL;DR** — 编译 → `serve --skip-validation` → 浏览器加账号 → 配环境变量 → `claude`。

### 第 1 步：编译

需要 **Go 1.25+**。项目是纯 Go（SQLite 用 `modernc` 纯 Go 驱动），**无需任何 C 工具链**：

```bash
git clone https://github.com/variyaone/JoyCode2api-VABoost.git
cd JoyCode2api-VABoost
CGO_ENABLED=0 go build -o JoyCode2Api ./cmd/JoyCode2Api/
```

前端已随仓库提交（`cmd/JoyCode2Api/static/`），不装 Node.js 也能编出带界面的二进制。只有要改前端时才需要 `cd web && npm install && npm run build`。

### 第 2 步：首次启动

> ⚠️ **两个必看的坑**
>
> 1. `./JoyCode2Api`（不带子命令）**只会打印帮助然后退出**，不会启动服务。必须用 `./JoyCode2Api serve`。
> 2. 首次启动时本机**还没有任何 JoyCode 凭据**（没装 JoyCode IDE / 非 macOS / Docker），`serve` 默认会去本地找凭据，找不到就直接报错退出。**第一次启动必须加 `--skip-validation`**，让服务先跑起来，凭据后面在 Dashboard 里加。

```bash
./JoyCode2Api serve --skip-validation --tls=false
```

看到 banner 后，浏览器打开 <http://localhost:34891>。

> **macOS 且已装 JoyCode IDE**：可以不加 `--skip-validation`，程序会自动从 `~/Library/Application Support/JoyCode/User/globalStorage/state.vscdb` 读取已登录凭据。其他平台首次启动一律加 `--skip-validation`。

### 第 3 步：配置 Dashboard

1. 首次访问进入初始化页面，设置 root 密码（≥ 6 位）。这是 Dashboard 登录密码，跟 JoyCode 账号无关。
2. 登录后在「账号管理」添加 JoyCode 账号，三种方式任选：

| 方式 | 操作 | 适用场景 |
|------|------|----------|
| **扫码登录**（推荐） | 点「扫码添加」，用**京东 App**（不是 JoyCode）扫二维码，手机确认后自动入库 | 有京东 App、最简单 |
| **OAuth 授权** | 点「OAuth授权登录」，跳转 JoyCode 页面完成授权 | 浏览器能访问 JoyCode |
| **手动添加** | 直接填 `pt_key` + `user_id` | 已有凭据，从 `state.vscdb` 或 OAuth 回调 URL 取 |

**OAuth 在 Docker / 远程部署时**：浏览器会跳转到一个打不开的 `localhost` 页面，这是正常的。把地址栏里完整的 URL（形如 `http://127.0.0.1:34891/?pt_key=xxx&...`）复制下来，粘进弹窗输入框，点「提交授权」。

### 第 4 步：接到编程工具

```bash
# Claude Code
export ANTHROPIC_BASE_URL=http://localhost:34891
export ANTHROPIC_API_KEY=sk-joy-xxxx   # Dashboard 里显示的 API Key；或偷懒用 joycode
claude

# Cursor / Codex（OpenAI 协议）
export OPENAI_BASE_URL=http://localhost:34891/v1
export OPENAI_API_KEY=sk-joy-xxxx
```

---

## 配置

### 启动参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `-H, --host` | `0.0.0.0` | 绑定地址 |
| `-p, --port` | `34891` | 监听端口 |
| `--tls` | `true` | 启用 HTTPS（自签名证书），同时仍接受 HTTP |
| `--skip-validation` | `false` | 跳过本地凭据检测，**非 macOS 首次启动必加** |
| `-k, --ptkey` | _空_ | 手动指定 JoyCode ptKey（留空则自动检测） |
| `-u, --userid` | _空_ | 手动指定 JoyCode userID（留空则自动检测） |
| `-v, --verbose` | `false` | 启用调试日志 |

### Dashboard 设置项

| 分组 | 设置项 | 默认 | 说明 |
|------|--------|------|------|
| **模型配置** | `default_model` | `JoyAI-Code` | 客户端未指定模型且账号未配置时的兜底模型 |
| | `default_max_tokens` | `8192` | 客户端未指定 `max_tokens` 时的默认值 |
| **连接优化** | `max_retries` | `3` | 请求失败自动重试次数 |
| | `request_timeout` | `120` | 与 JoyCode 后端通信超时（秒），低于 60 自动调到 60 |
| | `max_connections` | `20` | 与后端最大并发 HTTP 连接数，10 秒内生效 |
| **日志与监控** | `enable_request_logging` | `true` | 记录每个请求详情，关闭后「数据概览」无数据 |
| | `log_retention_days` | `30` | 请求日志保留天数，每小时自动清理，`0` 永久保留 |

每个账号还可以单独设置 `default_model`，优先级高于全局默认。

---

## 部署

```bash
# nohup 后台运行
nohup ./JoyCode2Api serve --skip-validation > joycode.log 2>&1 &

# 或装成系统服务
./JoyCode2Api service install
```

Docker 部署时用 `JOYCODE_STATE_DB` 环境变量挂载 `state.vscdb`（如需本地凭据）。

---

## API 参考

### 代理端点

| 端点 | 协议 | 说明 |
|------|------|------|
| `POST /v1/messages` | Anthropic | Claude Code |
| `POST /v1/chat/completions` | OpenAI | Cursor / Codex |
| `POST /v1/web-search` | — | 联网搜索 |
| `POST /v1/rerank` | — | 重排序 |
| `GET /v1/models` | OpenAI | 模型列表 |

### Dashboard 端点（JWT 鉴权）

`/api/stats`、`/api/costs`、`/api/usage-activity`（日期 × 小时用量）、`/api/accounts`、`/api/settings`、`/api/health`、`/api/github-stars`（双平台星数缓存）等，登录 Dashboard 后自动使用。

---

## FAQ

<details>
<summary><b>启动没有任何输出，服务也没运行</b></summary>

执行了 `./JoyCode2Api` 但没加 `serve` 子命令——根命令只会打印帮助然后退出。正确：`./JoyCode2Api serve --skip-validation --tls=false`
</details>

<details>
<summary><b>启动报 "cannot auto-detect credentials" 然后退出</b></summary>

非 macOS / 没装 JoyCode IDE 的环境找不到本地凭据。加 `--skip-validation`，凭据在 Dashboard 里扫码 / OAuth 添加。
</details>

<details>
<summary><b>Dashboard 打不开 / 要求登录</b></summary>

首次访问进入初始化页面设置 root 密码（≥ 6 位）。忘了密码用 `./JoyCode2Api reset-password` 重置。
</details>

<details>
<summary><b>OAuth 登录跳转到打不开的 localhost 页面</b></summary>

Docker / 远程部署的正常现象。复制地址栏完整 URL，粘进 Dashboard 弹窗，点「提交授权」。
</details>

<details>
<summary><b>费用数字是什么口径？</b></summary>

参考费用 = 已记录 Token × 模型公开单价（未知价格的请求单独计数，不当免费）；非实际账单。每日数据与账本核对，日志缺失的时段标「记录不足」，不填零。
</details>

<details>
<summary><b>GLM 的图标为什么是 Z.ai？</b></summary>

智谱对外的品牌是 Z.ai（bigmodel.cn 同源），VABoost 用 Z.ai 标识 GLM 系列模型。
</details>

---

## 使用限制

- 每个用户最多配置 **10 个账号**，超出限制将无法添加或导入
- 使用前请确保你已了解并遵守 JoyCode 的服务条款
- 如果觉得 JoyCode 的模型好用，建议去 [JoyCode 官方](https://joycode.jd.com/) 支持正版

---

## 贡献

欢迎提 Issue 和 PR。提 PR 前请：

1. Fork 仓库并拉取最新 `vaboost/main`
2. 新建分支：`git checkout -b feat/your-feature`
3. 确保后端 `go build ./...`、`go test ./...` 通过；改前端需 `cd web && npm run build` 重新构建产物
4. 提交时遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范

## 上游与致谢

本项目基于 [vibe-coding-labs / JoyCode2Api](https://github.com/vibe-coding-labs/JoyCode2Api) 独立改造，感谢上游维护者 [CC11001100](https://github.com/CC11001100) 及贡献者。上游源码版本 0.6.1，基线提交 [176ca9d（2026-07-15）](https://github.com/vibe-coding-labs/JoyCode2Api/commit/176ca9d4d7f98e46a5d979be4cd61b4bc0470b0f)。保留 Apache-2.0 许可证及原有版权声明，独立界面不代表上游背书。

## 许可证

[Apache 2.0](./LICENSE)
