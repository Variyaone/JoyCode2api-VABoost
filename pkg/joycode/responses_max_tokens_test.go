package joycode

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestResponsesExplicitMaxTokens(t *testing.T) {
	for _, tokens := range []int{1, 1024, 4095, 4096, 64000} {
		for _, effort := range []string{"none", "low", "max"} {
			for _, fromJSON := range []bool{false, true} {
				t.Run(fmt.Sprintf("tokens=%d/effort=%s/json=%t", tokens, effort, fromJSON), func(t *testing.T) {
					chat := map[string]interface{}{
						"model": "GPT-6 Astra", "max_tokens": tokens, "reasoning_effort": effort,
					}
					if fromJSON {
						data, err := json.Marshal(chat)
						if err != nil {
							t.Fatal(err)
						}
						if err := json.Unmarshal(data, &chat); err != nil {
							t.Fatal(err)
						}
					}
					got := ChatToResponses(chat)
					if got["max_output_tokens"] != tokens {
						t.Fatalf("explicit limit lost: max_output_tokens=%v, want %d", got["max_output_tokens"], tokens)
					}
					if reasoning, ok := got["reasoning"].(map[string]interface{}); !ok || reasoning["effort"] != effort {
						t.Fatalf("effort changed: %v", got["reasoning"])
					}
					if _, exists := got["max_tokens"]; exists {
						t.Fatal("Chat-only max_tokens leaked into Responses body")
					}
				})
			}
		}
	}
}

func TestResponsesOmitUnspecifiedOrInvalidMaxTokens(t *testing.T) {
	for _, effort := range []string{"none", "low", "max"} {
		for _, tc := range []struct {
			name  string
			value interface{}
		}{
			{"absent", nil}, {"zero", 0}, {"negative", -1},
			{"zero JSON", float64(0)}, {"negative JSON", float64(-1)},
			{"fraction JSON", 0.5}, {"fraction above one JSON", 1024.5},
			{"string", "1024"},
		} {
			t.Run(effort+"/"+tc.name, func(t *testing.T) {
				chat := map[string]interface{}{"model": "GPT-6 Astra", "reasoning_effort": effort}
				if tc.value != nil {
					chat["max_tokens"] = tc.value
				}
				if got := ChatToResponses(chat); got["max_output_tokens"] != nil {
					t.Fatalf("invented output limit: %v", got)
				}
			})
		}
	}
}
