package openai

import "testing"

func TestEffortFallbackRewritesOnlyTheNoneEffort(t *testing.T) {
	astraErr := `{"error":{"message":"Unsupported value: 'none' is not supported with the 'gpt-6-astra-2026-09-03' model. Supported values are: 'low', 'medium', 'high', 'xhigh', and 'max'."}}`

	t.Run("none is repaired to the lowest accepted effort", func(t *testing.T) {
		body := map[string]interface{}{
			"model":     "GPT-6 Astra",
			"reasoning": map[string]interface{}{"effort": "none"},
		}
		effort, ok := effortFallback(astraErr, body)
		if !ok {
			t.Fatal("fallback not applied")
		}
		if effort != "low" {
			t.Fatalf("effort = %q, want low", effort)
		}
		reasoning := body["reasoning"].(map[string]interface{})
		if reasoning["effort"] != "low" {
			t.Fatalf("body effort = %v, want low", reasoning["effort"])
		}
	})

	t.Run("a caller's own effort is left alone", func(t *testing.T) {
		body := map[string]interface{}{
			"model":     "GPT-6 Astra",
			"reasoning": map[string]interface{}{"effort": "max"},
		}
		if _, ok := effortFallback(astraErr, body); ok {
			t.Fatal("repaired an effort the caller named")
		}
		reasoning := body["reasoning"].(map[string]interface{})
		if reasoning["effort"] != "max" {
			t.Fatalf("body effort = %v, want max", reasoning["effort"])
		}
	})

	t.Run("unrelated errors do not touch the body", func(t *testing.T) {
		body := map[string]interface{}{
			"model":     "GPT-6 Astra",
			"reasoning": map[string]interface{}{"effort": "none"},
		}
		if _, ok := effortFallback(`{"error":{"message":"model overloaded"}}`, body); ok {
			t.Fatal("repaired on an unrelated error")
		}
		reasoning := body["reasoning"].(map[string]interface{})
		if reasoning["effort"] != "none" {
			t.Fatalf("body effort = %v, want none", reasoning["effort"])
		}
	})
}
