package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
)

func effortJSON(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEffortSDKLevelsSurviveTranslation(t *testing.T) {
	models := []string{"Claude-Opus-5", "Claude-Opus-4.8", "GPT-6 Astra", "GPT-5.6 Sol", "JoyAI-Code-1.5", "GLM-5.3", "GLM-5.2-jcloud", "Kimi-K3", "Kimi-K3-jcloud", "DeepSeek-V4-Pro", "MiniMax-M3", "Doubao-Seed-2.0-pro"}
	for _, model := range models {
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max", "invalid-audit-effort"} {
			t.Run(model+"/"+effort, func(t *testing.T) {
				var req MessageRequest
				payload := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"test"}],"thinking":{"type":"adaptive"},"output_config":{"effort":%q},"max_tokens":8192}`, model, effort)
				if err := json.Unmarshal([]byte(payload), &req); err != nil {
					t.Fatal(err)
				}
				body := TranslateRequest(&req, "", "")
				switch {
				case IsNativeAnthropicModel(model):
					body = effortJSON(t, TranslateAnthropicRequest(&req, "", ""))
					if body["output_config"].(map[string]interface{})["effort"] != effort {
						t.Fatalf("native effort lost: %v", body)
					}
					if !reflect.DeepEqual(body["thinking"], map[string]interface{}{"type": "adaptive"}) {
						t.Fatalf("adaptive altered or budget invented: %v", body["thinking"])
					}
				case joycode.IsResponsesAPIModel(model):
					body = effortJSON(t, joycode.ChatToResponses(body))
					if body["reasoning"].(map[string]interface{})["effort"] != effort {
						t.Fatalf("GPT effort lost: %v", body)
					}
					for _, key := range []string{"thinking", "output_config", "reasoning_effort", "budget_tokens"} {
						if _, found := body[key]; found {
							t.Errorf("GPT got foreign parameter %s", key)
						}
					}
				default:
					body = effortJSON(t, body)
					if body["reasoning_effort"] != effort {
						t.Fatalf("chat effort lost or downgraded: %v", body)
					}
					wantThinking := "enabled"
					if model == "MiniMax-M3" {
						wantThinking = "adaptive"
					}
					if !reflect.DeepEqual(body["thinking"], map[string]interface{}{"type": wantThinking}) {
						t.Fatalf("chat thinking must follow provider schema without budget: %v", body)
					}
				}
			})
		}
	}
}

func TestEffortNativePreservesRawConfigAndContent(t *testing.T) {
	payload := `{
		"model":"Claude-Opus-5", "max_tokens":8192,
		"thinking":{"type":"adaptive","display":"summarized","future_option":{"n":9007199254740993}},
		"output_config":{"effort":"max","format":{"type":"json_schema","schema":{"type":"object","properties":{"n":{"const":9007199254740993}}}},"future_option":{"enabled":false,"items":[1,null,"x"]}},
		"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"prior","signature":"raw-signature"},{"type":"text","text":"prior reply","citations":[{"unknown":"keep"}]}]},{"role":"user","content":[{"type":"text","text":"next"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]}],
		"tool_choice":{"type":"auto","disable_parallel_tool_use":true},
		"tools":[{"name":"check","input_schema":{"type":"object","properties":{"n":{"const":9007199254740993}}}}]
	}`
	var req MessageRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatal(err)
	}
	want := effortJSON(t, json.RawMessage(payload))
	got := effortJSON(t, TranslateAnthropicRequest(&req, "", ""))
	roundTrip := effortJSON(t, req)
	for _, key := range []string{"thinking", "output_config", "messages", "tool_choice", "tools"} {
		if !reflect.DeepEqual(got[key], want[key]) || !reflect.DeepEqual(roundTrip[key], want[key]) {
			t.Errorf("%s changed: native=%v roundtrip=%v want=%v", key, got[key], roundTrip[key], want[key])
		}
	}
}

func TestEffortMissingDisabledAndBudget(t *testing.T) {
	for _, tc := range []struct {
		name, config, nativeType, chatType, gptEffort string
		budget                                        int
	}{
		{name: "missing", config: ``, nativeType: "disabled"},
		{name: "null", config: `,"thinking":null,"output_config":null`, nativeType: "disabled"},
		{name: "disabled", config: `,"thinking":{"type":"disabled"}`, nativeType: "disabled", chatType: "disabled", gptEffort: "none"},
		{name: "adaptive no effort", config: `,"thinking":{"type":"adaptive"}`, nativeType: "adaptive", chatType: "enabled"},
		{name: "explicit adaptive budget is not silently repaired", config: `,"thinking":{"type":"adaptive","budget_tokens":4096}`, nativeType: "adaptive", chatType: "enabled", budget: 4096},
		{name: "enabled budget no effort", config: `,"thinking":{"type":"enabled","budget_tokens":4096}`, nativeType: "enabled", chatType: "enabled", budget: 4096},
		{name: "format only", config: `,"output_config":{"format":{"type":"json_schema","schema":{"type":"object"}}}`, nativeType: "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req MessageRequest
			if err := json.Unmarshal([]byte(`{"model":"Claude-Opus-5","max_tokens":8192`+tc.config+`}`), &req); err != nil {
				t.Fatal(err)
			}
			native := effortJSON(t, TranslateAnthropicRequest(&req, "", ""))
			thinking := native["thinking"].(map[string]interface{})
			if thinking["type"] != tc.nativeType {
				t.Fatalf("native thinking=%v", thinking)
			}
			if tc.budget > 0 {
				if thinking["budget_tokens"] != json.Number(fmt.Sprint(tc.budget)) {
					t.Fatalf("native budget lost: %v", thinking)
				}
			} else if _, exists := thinking["budget_tokens"]; exists {
				t.Fatalf("invented zero budget: %v", thinking)
			}
			if req.OutputConfig == nil {
				if _, exists := native["output_config"]; exists {
					t.Fatal("invented output_config")
				}
			}
			req.Model = "GLM-5.3"
			chat := effortJSON(t, TranslateRequest(&req, "", ""))
			if tc.chatType == "" {
				if _, exists := chat["thinking"]; exists {
					t.Fatal("invented chat thinking")
				}
			} else if !reflect.DeepEqual(chat["thinking"], map[string]interface{}{"type": tc.chatType}) {
				t.Fatalf("chat thinking/budget=%v", chat["thinking"])
			}
			if _, exists := chat["reasoning_effort"]; exists {
				t.Fatal("invented effort from thinking or token budget")
			}
			gpt := effortJSON(t, joycode.ChatToResponses(chat))
			if tc.gptEffort == "" {
				if _, exists := gpt["reasoning"]; exists {
					t.Fatalf("invented GPT reasoning: %v", gpt)
				}
			} else if gpt["reasoning"].(map[string]interface{})["effort"] != tc.gptEffort {
				t.Fatalf("disabled not mapped: %v", gpt)
			}
		})
	}
}

func TestEffortConfigReuseClearsPreviousFields(t *testing.T) {
	var output OutputConfig
	var thinking ThinkingConfig
	if err := json.Unmarshal([]byte(`{"effort":"max","format":{"type":"json_schema"},"future":true}`), &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"type":"enabled","budget_tokens":4096,"future":true}`), &thinking); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{}`), &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"type":"adaptive"}`), &thinking); err != nil {
		t.Fatal(err)
	}
	if len(effortJSON(t, output)) != 0 || !reflect.DeepEqual(effortJSON(t, thinking), map[string]interface{}{"type": "adaptive"}) {
		t.Fatal("reused config retained previous effort/budget/unknown fields")
	}
}

func TestEffortKnownCompletionModelDoesNotFallback(t *testing.T) {
	if got := resolveModel("JoyCode-Base-V3", "GPT-6 Astra", "Claude-Opus-5"); got != "JoyCode-Base-V3" {
		t.Fatalf("completion model disguised as %s", got)
	}
	if got := resolveModel("desktop-alias", "GPT-6 Astra", "Claude-Opus-5"); got != "GPT-6 Astra" {
		t.Fatalf("existing desktop alias fallback changed: %s", got)
	}
}

// Exercise SDK JSON -> handler -> actual serialized transport body, using only
// an in-memory RoundTripper. No production process or upstream API is contacted.
func TestEffortSDKHandlerOutboundBody(t *testing.T) {
	for _, model := range []string{"Claude-Opus-5", "Claude-Opus-4.8", "GPT-6 Astra", "GPT-5.6 Sol", "Doubao-Seed-2.0-pro"} {
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
			for _, stream := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", model, effort, stream), func(t *testing.T) {
					client := joycode.NewClient("synthetic", "synthetic")
					calls := 0
					client.SetHTTPClient(&http.Client{Transport: effortRoundTripper(func(r *http.Request) (*http.Response, error) {
						calls++
						var body map[string]interface{}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						function := r.URL.Query().Get("functionId")
						response := ""
						switch {
						case IsNativeAnthropicModel(model):
							if function != "anthropic_completions" || body["output_config"].(map[string]interface{})["effort"] != effort {
								t.Fatalf("wrong native endpoint/effort: %s %v", function, body)
							}
							if !reflect.DeepEqual(body["thinking"], map[string]interface{}{"type": "adaptive"}) {
								t.Fatalf("adaptive thinking lost: %v", body)
							}
							response = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_fixture\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
						case joycode.IsResponsesAPIModel(model):
							if function != "responses_completions" || body["reasoning"].(map[string]interface{})["effort"] != effort {
								t.Fatalf("wrong GPT endpoint/effort: %s %v", function, body)
							}
							if body["thinking"] != nil || body["output_config"] != nil {
								t.Fatalf("GPT got Claude parameters: %v", body)
							}
							response = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
						default:
							if function != "chat_completions" || body["reasoning_effort"] != effort || !reflect.DeepEqual(body["thinking"], map[string]interface{}{"type": "enabled"}) {
								t.Fatalf("wrong chat endpoint/effort: %s %v", function, body)
							}
							if stream {
								response = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
							} else {
								response = `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`
							}
						}
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
					})})
					mux := http.NewServeMux()
					NewHandler(client, nil).RegisterRoutes(mux)
					payload := fmt.Sprintf(`{"model":%q,"stream":%t,"max_tokens":8192,"messages":[{"role":"user","content":"test"}],"thinking":{"type":"adaptive"},"output_config":{"effort":%q}}`, model, stream, effort)
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
					if calls != 1 || w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"ok"`) {
						t.Fatalf("calls=%d status=%d response=%s", calls, w.Code, w.Body.String())
					}
				})
			}
		}
	}
}

type effortRoundTripper func(*http.Request) (*http.Response, error)

func (f effortRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
