package pricing

// Rate uses tenths of a micro-USD per token; numerically the same as
// tenths of USD per million tokens. RMB rates are converted from the
// official CNY list price using the fixed reference rate below.
type Rate struct {
	Model  string `json:"model"`
	Input  *int64 `json:"input_tenth_micro_usd"`
	Output *int64 `json:"output_tenth_micro_usd"`
	Source string `json:"source"`
	URL    string `json:"url"`
	Note   string `json:"note"`
}

const Version = "public-base-2026-09-14-v4"
const CollectedAt = "2026-09-14"

func n(v int64) *int64 { return &v }

// Reference rate for display conversion only; not a live FX quote.
const USDToC = 7.2 // CNY per USD

// cny converts yuan per 1M tokens into tenths of micro-USD per token.
// yuan CNY per 1M tokens / 7.2 = USD per 1M tokens; 1 USD/1M tokens equals
// 10 tenths-of-micro-USD per token, so yuan * 10 / 7.2 = yuan * 100 / 72.
func cny(yuan int64) *int64 { return n(yuan * 100 / 72) }

// Prices are reference base API rates, NOT JoyCode invoices. jcloud models
// inherit the vendor list price of the same base model and are marked as such.
var Rates = []Rate{
	{"GPT-6 Astra", n(100), n(500), "Artificial Analysis", "https://artificialanalysis.ai/models/gpt-6-astra", "公开基础价参考；非 JoyCode 账单"},
	{"GPT-5.6 Sol", n(40), n(200), "Artificial Analysis", "https://artificialanalysis.ai/models/gpt-5-6-sol", "未计长上下文/服务层级等附加费"},
	{"Claude-Opus-5", n(50), n(250), "Claude 官方", "https://platform.claude.com/docs/en/about-claude/pricing", "第一方标准价；非 Bedrock 区域账单"},
	{"Claude-Opus-4.8", n(50), n(250), "Claude 官方", "https://platform.claude.com/docs/en/about-claude/pricing", "第一方标准价"},
	{"Claude-Opus-4.7", n(50), n(250), "Claude 官方", "https://platform.claude.com/docs/en/about-claude/pricing", "历史型号参考价"},
	{"Claude-Opus-4.6", n(50), n(250), "Claude 官方", "https://platform.claude.com/docs/en/about-claude/pricing", "历史型号参考价"},
	{"Claude-Sonnet-4.6", n(30), n(150), "Claude 官方", "https://platform.claude.com/docs/en/about-claude/pricing", "历史型号参考价"},
	{"GLM-5.3", cny(8), cny(28), "智谱官网", "https://docs.bigmodel.cn/cn/guide/start/pricing", "输入未命中缓存价 ¥8/¥28；缓存命中 ¥2 未计"},
	{"GLM-5.2-jcloud", cny(8), cny(28), "智谱官网 GLM-5.2", "https://docs.bigmodel.cn/cn/guide/start/pricing", "按同版本 GLM-5.2 官网价参考；非 jcloud 实际账单"},
	{"Kimi-K3", cny(20), cny(100), "Kimi 官网", "https://platform.kimi.com/docs/pricing/chat-k3", "输入未命中缓存价；缓存命中 ¥2 未计"},
	{"Kimi-K3-jcloud", cny(20), cny(100), "Kimi 官网", "https://platform.kimi.com/docs/pricing/chat-k3", "按 Kimi 官网价参考；非 jcloud 实际账单"},
	{"DeepSeek-V4-Pro", n(66), n(198), "DeepSeek 官网", "https://api-docs.deepseek.com/quick_start/pricing/", "V4-Pro-0813 版 peak 价；off-peak 半价未分桶，按 peak 估算"},
	{"MiniMax-M3", n(3), n(12), "Artificial Analysis", "https://artificialanalysis.ai/models/minimax-m3", "公开基础价参考"},
	{"Doubao-Seed-2.0-pro", cny(48), cny(240), "火山方舟", "https://www.volcengine.com/docs/82379/1544106", "取输入长度 (32,128] 阶梯价；日志无长度分桶"},
	{"JoyAI-Code-1.5", nil, nil, "", "", "本次未取得可信匹配价格"},
	{"JoyCode-Base-V3", nil, nil, "", "", "本次未取得可信匹配价格"},
}

func Lookup(model string) Rate {
	aliases := map[string]string{
		"Claude-Opus-5-hq": "Claude-Opus-5", "Claude-Opus-4.8-hq": "Claude-Opus-4.8",
		"Claude-Opus-4.7-hq": "Claude-Opus-4.7", "Claude-Opus-4.6-hq": "Claude-Opus-4.6", "Claude-Sonnet-4.6-hq": "Claude-Sonnet-4.6",
	}
	if alias, ok := aliases[model]; ok {
		model = alias
	}
	for _, r := range Rates {
		if r.Model == model {
			return r
		}
	}
	return Rate{Model: model, Note: "无可核实单价"}
}
