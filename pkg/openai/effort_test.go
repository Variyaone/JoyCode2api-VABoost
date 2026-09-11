package openai

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
)

func TestEffortOpenAIRequestFiveLevels(t *testing.T) {
	for _, model := range []string{"GPT-6 Astra", "GPT-5.6 Sol", "JoyAI-Code-1.5", "GLM-5.3", "GLM-5.2-jcloud", "Kimi-K3", "Kimi-K3-jcloud", "DeepSeek-V4-Pro", "MiniMax-M3", "Doubao-Seed-2.0-pro"} {
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max", "invalid-audit-effort"} {
			for _, field := range []string{"reasoning_effort", "reasoning"} {
				t.Run(model+"/"+effort+"/"+field, func(t *testing.T) {
					value := fmt.Sprintf(`%q`, effort)
					if field == "reasoning" {
						value = fmt.Sprintf(`{"effort":%q,"summary":"auto"}`, effort)
					}
					var req ChatRequest
					payload := fmt.Sprintf(`{"model":%q,%q:%s,"thinking":{"type":"adaptive","budget_tokens":4096},"messages":[{"role":"user","content":[{"type":"text","text":"test"}]}]}`, model, field, value)
					if err := json.Unmarshal([]byte(payload), &req); err != nil {
						t.Fatal(err)
					}
					body := TranslateRequest(&req)
					if body["reasoning_effort"] != effort {
						t.Fatalf("effort lost: %v", body)
					}
					wantThinking := "enabled"
					if model == "MiniMax-M3" {
						wantThinking = "adaptive"
					}
					if !reflect.DeepEqual(body["thinking"], map[string]interface{}{"type": wantThinking}) {
						t.Fatalf("provider thinking/budget mapping wrong: %v", body)
					}
					if joycode.IsResponsesAPIModel(model) {
						body = joycode.ChatToResponses(body)
						reasoning := body["reasoning"].(map[string]interface{})
						if reasoning["effort"] != effort || (field == "reasoning" && reasoning["summary"] != "auto") {
							t.Fatalf("reasoning lost: %v", body)
						}
						if body["thinking"] != nil || body["reasoning_effort"] != nil {
							t.Fatalf("foreign GPT parameters: %v", body)
						}
					} else if body["reasoning"] != nil {
						t.Fatalf("Responses-specific config leaked to chat: %v", body)
					}
				})
			}
		}
	}
}

func TestEffortOpenAIMissingAndExplicitControls(t *testing.T) {
	for _, tc := range []struct {
		name, model, fields, effort, thinking string
	}{
		{"missing", "GPT-6 Astra", ``, "", ""},
		{"thinking only does not invent effort", "GLM-5.3", `,"thinking":{"type":"enabled","budget_tokens":4096}`, "", "enabled"},
		{"Doubao effort enables thinking", "Doubao-Seed-2.0-pro", `,"reasoning_effort":"max"`, "max", "enabled"},
		{"Doubao missing does not enable", "Doubao-Seed-2.0-pro", ``, "", ""},
		{"Doubao explicit disabled wins", "Doubao-Seed-2.0-pro", `,"reasoning_effort":"xhigh","thinking":{"type":"disabled"}`, "xhigh", "disabled"},
		{"chat effort takes precedence", "GPT-6 Astra", `,"reasoning_effort":"high","reasoning":{"effort":"low","summary":"auto"}`, "high", ""},
		{"reasoning summary only", "GPT-6 Astra", `,"reasoning":{"summary":"auto"}`, "", ""},
		{"disabled GPT", "GPT-6 Astra", `,"thinking":{"type":"disabled"}`, "", "disabled"},
		{"raw null", "Doubao-Seed-2.0-pro", `,"thinking":null,"reasoning":null`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req ChatRequest
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"model":%q%s}`, tc.model, tc.fields)), &req); err != nil {
				t.Fatal(err)
			}
			body := TranslateRequest(&req)
			if tc.effort == "" {
				if _, found := body["reasoning_effort"]; found {
					t.Fatalf("invented effort: %v", body)
				}
			} else if body["reasoning_effort"] != tc.effort {
				t.Fatalf("effort=%v want=%s", body["reasoning_effort"], tc.effort)
			}
			if tc.thinking == "" {
				if _, found := body["thinking"]; found {
					t.Fatalf("invented thinking: %v", body)
				}
			} else if !reflect.DeepEqual(body["thinking"], map[string]interface{}{"type": tc.thinking}) {
				t.Fatalf("thinking=%v want=%s", body["thinking"], tc.thinking)
			}
			if tc.name == "disabled GPT" {
				gpt := joycode.ChatToResponses(body)
				if gpt["reasoning"].(map[string]interface{})["effort"] != "none" {
					t.Fatalf("GPT disabled lost: %v", gpt)
				}
			}
		})
	}
}

func TestEffortOpenAIKnownCompletionModelDoesNotFallback(t *testing.T) {
	if got := ResolveModel("JoyCode-Base-V3", "GPT-6 Astra", "Claude-Opus-5"); got != "JoyCode-Base-V3" {
		t.Fatalf("completion model disguised as %s", got)
	}
	if got := ResolveModel("desktop-alias", "GPT-6 Astra", "Claude-Opus-5"); got != "GPT-6 Astra" {
		t.Fatalf("desktop fallback changed: %s", got)
	}
}
