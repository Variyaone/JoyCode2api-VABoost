package joycode

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEffortResponsesPrecedenceAndRawReasoning(t *testing.T) {
	for _, tc := range []struct {
		name string
		body map[string]interface{}
		want map[string]interface{}
	}{
		{"missing", map[string]interface{}{}, nil},
		{"adaptive no effort", map[string]interface{}{"thinking": json.RawMessage(`{"type":"adaptive"}`)}, nil},
		{"budget is not effort", map[string]interface{}{"thinking": map[string]interface{}{"type": "enabled", "budget_tokens": 10000}}, nil},
		{"disabled", map[string]interface{}{"thinking": json.RawMessage(`{"type":"disabled"}`)}, map[string]interface{}{"effort": "none"}},
		{"raw output config", map[string]interface{}{"output_config": json.RawMessage(`{"effort":"max","format":{"type":"json_schema"},"future":true}`)}, map[string]interface{}{"effort": "max"}},
		{"typed output config", map[string]interface{}{"output_config": struct {
			Effort string `json:"effort"`
		}{"xhigh"}}, map[string]interface{}{"effort": "xhigh"}},
		{"format without effort", map[string]interface{}{"output_config": json.RawMessage(`{"format":{"type":"json_schema"}}`)}, nil},
		{"reasoning raw extras", map[string]interface{}{"reasoning": json.RawMessage(`{"effort":"medium","summary":"auto","future":{"n":9007199254740993}}`)}, map[string]interface{}{"effort": "medium", "summary": "auto", "future": map[string]interface{}{"n": json.Number("9007199254740993")}}},
		{"reasoning map", map[string]interface{}{"reasoning": map[string]interface{}{"effort": "high", "summary": "detailed"}}, map[string]interface{}{"effort": "high", "summary": "detailed"}},
		{"reasoning beats output config", map[string]interface{}{"reasoning": json.RawMessage(`{"effort":"low","summary":"auto"}`), "output_config": map[string]interface{}{"effort": "max"}}, map[string]interface{}{"effort": "low", "summary": "auto"}},
		{"chat effort wins", map[string]interface{}{"reasoning_effort": "xhigh", "reasoning": json.RawMessage(`{"effort":"low","summary":"auto"}`), "output_config": map[string]interface{}{"effort": "max"}}, map[string]interface{}{"effort": "xhigh", "summary": "auto"}},
		{"fill missing effort", map[string]interface{}{"reasoning": json.RawMessage(`{"summary":"auto"}`), "output_config": map[string]interface{}{"effort": "max"}}, map[string]interface{}{"effort": "max", "summary": "auto"}},
		{"explicit effort over thinking", map[string]interface{}{"thinking": json.RawMessage(`{"type":"disabled"}`), "reasoning_effort": "low"}, map[string]interface{}{"effort": "low"}},
		{"invalid retained", map[string]interface{}{"reasoning_effort": "invalid-audit-effort"}, map[string]interface{}{"effort": "invalid-audit-effort"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			body := ChatToResponses(tc.body)
			if tc.want == nil {
				if _, present := body["reasoning"]; present {
					t.Fatalf("invented reasoning: %v", body)
				}
			} else if !reflect.DeepEqual(body["reasoning"], tc.want) {
				t.Fatalf("reasoning=%v, want %v", body["reasoning"], tc.want)
			}
			for _, key := range []string{"thinking", "output_config", "reasoning_effort", "budget_tokens"} {
				if _, exists := body[key]; exists {
					t.Errorf("foreign parameter %s leaked to GPT", key)
				}
			}
			after, err := json.Marshal(tc.body)
			if err != nil || string(before) != string(after) {
				t.Fatalf("translation mutated request: before=%s after=%s err=%v", before, after, err)
			}
		})
	}
}

func TestEffortResponsesFiveDistinctLevels(t *testing.T) {
	for _, model := range []string{"GPT-6 Astra", "GPT-5.6 Sol"} {
		seen := map[string]bool{}
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
			body := ChatToResponses(map[string]interface{}{"model": model, "output_config": map[string]interface{}{"effort": effort}})
			reasoning, _ := body["reasoning"].(map[string]interface{})
			if reasoning["effort"] != effort {
				t.Fatalf("%s %s mapped to %v", model, effort, reasoning)
			}
			seen[reasoning["effort"].(string)] = true
		}
		if len(seen) != 5 {
			t.Fatalf("levels collapsed: %v", seen)
		}
	}
}

func TestEffortChatThinkingCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, model, effort string
		thinking            interface{}
		want                map[string]interface{}
	}{
		{"missing", "GLM-5.3", "", nil, nil},
		{"effort only no invented switch", "Kimi-K3", "max", nil, nil},
		{"Doubao effort only", "Doubao-Seed-2.0-pro", "max", nil, map[string]interface{}{"type": "enabled"}},
		{"Doubao no effort", "Doubao-Seed-2.0-pro", "", nil, nil},
		{"Doubao null thinking", "Doubao-Seed-2.0-pro", "xhigh", json.RawMessage(`null`), map[string]interface{}{"type": "enabled"}},
		{"Doubao disabled wins", "Doubao-Seed-2.0-pro", "max", json.RawMessage(`{"type":"disabled"}`), map[string]interface{}{"type": "disabled"}},
		{"adaptive budget dropped", "GLM-5.3", "high", json.RawMessage(`{"type":"adaptive","budget_tokens":1000,"display":"summarized"}`), map[string]interface{}{"type": "enabled"}},
		{"MiniMax adaptive retained", "MiniMax-M3", "high", json.RawMessage(`{"type":"adaptive","budget_tokens":1000}`), map[string]interface{}{"type": "adaptive"}},
		{"enabled budget dropped", "DeepSeek-V4-Pro", "", map[string]interface{}{"type": "enabled", "budget_tokens": 4096}, map[string]interface{}{"type": "enabled"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChatThinking(tc.thinking, tc.model, tc.effort); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("thinking=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestEffortResponsesPreservesRawMessageParts(t *testing.T) {
	parts := `[{"type":"text","text":"inspect image"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]`
	var decoded []interface{}
	if err := json.Unmarshal([]byte(parts), &decoded); err != nil {
		t.Fatal(err)
	}
	typed := []map[string]interface{}{{"type": "text", "text": "inspect image"}, {"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,AA=="}}}
	for _, content := range []interface{}{json.RawMessage(parts), decoded, typed} {
		body := ChatToResponses(map[string]interface{}{
			"model": "GPT-6 Astra", "reasoning_effort": "max",
			"messages": []map[string]interface{}{{"role": "user", "content": content}},
		})
		input := body["input"].([]interface{})
		got := input[0].(map[string]interface{})["content"]
		want := []map[string]interface{}{{"type": "input_text", "text": "inspect image"}, {"type": "input_image", "image_url": "data:image/png;base64,AA=="}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%T parts lost with effort: got=%v want=%v", content, got, want)
		}
	}
}
