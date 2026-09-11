package joycode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChatToResponses converts an OpenAI chat-completions style body (model,
// messages, tools, ...) into the Responses API format that GPT-family models
// on the JoyCode platform require.
//   - the leading system/developer message becomes "instructions"; later ones
//     remain in input order (Claude Code uses these for mid-turn user messages)
//   - user text -> {role:user, content:[{type:input_text,text}]}
//   - assistant text -> {role:assistant, content:[{type:output_text,text}]}
//   - assistant tool_calls -> {type:function_call, call_id, name, arguments}
//   - tool role -> {type:function_call_output, call_id, output}
//   - tools flattened to Responses function schema
func ChatToResponses(chatBody map[string]interface{}) map[string]interface{} {
	body := map[string]interface{}{
		"model":  chatBody["model"],
		"stream": chatBody["stream"],
	}
	if reasoning := responsesReasoning(chatBody); len(reasoning) > 0 {
		body["reasoning"] = reasoning
	}
	// Honor every explicit positive output limit, including small budgets.
	// Reasoning may consume that budget before producing text, but removing
	// the limit would silently exceed the caller's requested token allowance.
	if mt, ok := chatBody["max_tokens"]; ok {
		switch v := mt.(type) {
		case int:
			if v > 0 {
				body["max_output_tokens"] = v
			}
		case float64:
			if tokens := int(v); tokens > 0 && float64(tokens) == v {
				body["max_output_tokens"] = tokens
			}
		}
	}

	// Normalize messages to []interface{} regardless of the concrete Go type
	// the caller used ([]map[string]interface{} from buildMessages, or
	// []interface{} from raw JSON unmarshaling).
	var msgs []interface{}
	switch m := chatBody["messages"].(type) {
	case []interface{}:
		msgs = m
	case []map[string]interface{}:
		msgs = make([]interface{}, len(m))
		for i, v := range m {
			msgs[i] = v
		}
	default:
		if raw, ok := chatBody["messages"]; ok && raw != nil {
			if b, err := json.Marshal(raw); err == nil {
				json.Unmarshal(b, &msgs)
			}
		}
	}
	input := make([]interface{}, 0, len(msgs))
	instructions := ""
	for i, mi := range msgs {
		m, ok := mi.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		switch role {
		case "system", "developer":
			if i == 0 {
				instructions = extractStringContent(m["content"])
			} else {
				input = append(input, map[string]interface{}{
					"role":    role,
					"content": chatContentToParts(m["content"], "input_text"),
				})
			}
			continue
		case "tool":
			callID, _ := m["tool_call_id"].(string)
			output := extractStringContent(m["content"])
			if output == "" {
				output = " "
			}
			input = append(input, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": TrimCallID(callID),
				"output":  output,
			})
			continue
		}

		text := extractStringContent(m["content"])
		switch role {
		case "user":
			input = append(input, map[string]interface{}{
				"role":    "user",
				"content": chatContentToParts(m["content"], "input_text"),
			})
		case "assistant":
			if strings.TrimSpace(text) != "" {
				input = append(input, map[string]interface{}{
					"role":    "assistant",
					"content": []map[string]interface{}{{"type": "output_text", "text": text}},
				})
			}
			if tcs, ok := m["tool_calls"].([]interface{}); ok {
				for _, tci := range tcs {
					tc, ok := tci.(map[string]interface{})
					if !ok {
						continue
					}
					fn, _ := tc["function"].(map[string]interface{})
					if fn == nil {
						continue
					}
					name, _ := fn["name"].(string)
					args, _ := fn["arguments"].(string)
					if args == "" || !json.Valid([]byte(args)) {
						args = "{}"
					}
					id, _ := tc["id"].(string)
					input = append(input, map[string]interface{}{
						"type":      "function_call",
						"call_id":   TrimCallID(id),
						"name":      name,
						"arguments": args,
					})
				}
			}
		default:
			input = append(input, map[string]interface{}{
				"role":    role,
				"content": chatContentToParts(m["content"], "input_text"),
			})
		}
	}
	body["input"] = input
	if instructions != "" {
		body["instructions"] = instructions
	}

	if tools, ok := chatBody["tools"].([]interface{}); ok {
		if rt := ConvertChatTools(tools); len(rt) > 0 {
			body["tools"] = rt
		}
	}
	if tc, ok := chatBody["tool_choice"]; ok {
		switch v := tc.(type) {
		case string:
			body["tool_choice"] = v
		case map[string]interface{}:
			if fn, ok := v["function"].(map[string]interface{}); ok {
				if name, ok := fn["name"].(string); ok {
					body["tool_choice"] = map[string]interface{}{"type": "function", "name": name}
				}
			}
		}
	}
	return body
}

// ReasoningEffort reads a Responses-style reasoning object's effort without
// coercing or normalizing its value. Callers can supply raw JSON or typed maps.
func ReasoningEffort(raw interface{}) string {
	effort, _ := jsonObject(raw)["effort"].(string)
	return effort
}

// ChatThinking translates only the shared on/off control, not Claude's token
// budget or adaptive-only options. Doubao requires enabled when effort is set;
// an explicit disabled always wins over that implicit default.
//
// This is parameter transport, NOT a promise of five distinct reasoning levels.
// The audit in tools/model-audit/upstream-matrix.json shows some chat providers
// even accept invalid effort values. Doubao may fold xhigh/max into high. Keep
// the caller's effort unchanged and leave validation/interpretation upstream.
func ChatThinking(raw interface{}, model, effort string) map[string]interface{} {
	thinking := jsonObject(raw)
	if typ, ok := thinking["type"].(string); ok && typ != "" {
		if typ == "adaptive" && !strings.HasPrefix(strings.ToLower(model), "minimax-") {
			typ = "enabled"
		}
		return map[string]interface{}{"type": typ}
	}
	if raw == nil || thinking == nil {
		if strings.EqualFold(model, "Doubao-Seed-2.0-pro") && effort != "" {
			return map[string]interface{}{"type": "enabled"}
		}
	}
	return nil
}

// responsesReasoning merges supported controls without leaking Anthropic
// thinking/output_config (or budget_tokens) into GPT requests. Explicit OpenAI
// reasoning_effort wins, then reasoning.effort, then output_config.effort. No
// effort is invented for enabled/adaptive or a numeric token budget. If effort
// is absent, explicit disabled maps to the Responses API's "none" value.
func responsesReasoning(chatBody map[string]interface{}) map[string]interface{} {
	reasoning := jsonObject(chatBody["reasoning"])
	if reasoning == nil {
		reasoning = map[string]interface{}{}
	}
	if effort, ok := chatBody["reasoning_effort"].(string); ok && effort != "" {
		reasoning["effort"] = effort
	} else if _, specified := reasoning["effort"]; !specified {
		if effort := ReasoningEffort(chatBody["output_config"]); effort != "" {
			reasoning["effort"] = effort
		} else if jsonObject(chatBody["thinking"])["type"] == "disabled" {
			reasoning["effort"] = "none"
		}
	}
	return reasoning
}

// jsonObject returns a detached object, preserving raw nested values (including
// large JSON numbers) and avoiding mutation of the caller's reasoning map.
func jsonObject(raw interface{}) map[string]interface{} {
	if object, ok := raw.(map[string]interface{}); ok {
		copy := make(map[string]interface{}, len(object))
		for key, value := range object {
			copy[key] = value
		}
		return copy
	}
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var object map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if decoder.Decode(&object) != nil {
		return nil
	}
	return object
}

// TrimCallID caps call IDs at 64 chars (upstream rejects longer IDs; the IDE
// keeps the tail).
func TrimCallID(id string) string {
	if len(id) <= 64 {
		return id
	}
	return id[len(id)-64:]
}

// ConvertChatTools converts OpenAI chat tools to Responses function schema.
func ConvertChatTools(tools []interface{}) []interface{} {
	out := make([]interface{}, 0, len(tools))
	for _, t := range tools {
		tm, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		fn, ok := tm["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}
		rt := map[string]interface{}{"type": "function", "name": name}
		if d, ok := fn["description"].(string); ok && d != "" {
			rt["description"] = d
		}
		// parameters may arrive as a map, or as json.RawMessage when passed
		// through from typed structs (e.g. anthropic Tool.InputSchema).
		params, ok := fn["parameters"].(map[string]interface{})
		if !ok {
			if raw, ok := fn["parameters"].(json.RawMessage); ok && len(raw) > 0 {
				var p map[string]interface{}
				if err := json.Unmarshal(raw, &p); err == nil {
					params = p
				}
			} else if s, ok := fn["parameters"].(string); ok && json.Valid([]byte(s)) {
				var p map[string]interface{}
				if err := json.Unmarshal([]byte(s), &p); err == nil {
					params = p
				}
			}
		}
		if params == nil {
			params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false}
		}
		rt["parameters"] = params
		if strict, ok := fn["strict"].(bool); ok {
			rt["strict"] = strict
		}
		out = append(out, rt)
	}
	return out
}

// normalizeChatContent handles typed Anthropic conversion and raw OpenAI parts
// alike; otherwise a []map or RawMessage image/text array becomes debug text.
func normalizeChatContent(raw interface{}) interface{} {
	switch content := raw.(type) {
	case json.RawMessage:
		var decoded interface{}
		if json.Unmarshal(content, &decoded) == nil {
			return decoded
		}
	case []map[string]interface{}:
		parts := make([]interface{}, len(content))
		for i, part := range content {
			parts[i] = part
		}
		return parts
	}
	return raw
}

func extractStringContent(raw interface{}) string {
	switch c := normalizeChatContent(raw).(type) {
	case nil:
		return ""
	case string:
		return c
	case []interface{}:
		var sb strings.Builder
		for _, p := range c {
			if pm, ok := p.(map[string]interface{}); ok {
				if t, _ := pm["type"].(string); t == "text" {
					if txt, _ := pm["text"].(string); txt != "" {
						if sb.Len() > 0 {
							sb.WriteString("\n")
						}
						sb.WriteString(txt)
					}
				}
			}
		}
		return sb.String()
	default:
		return fmt.Sprintf("%v", c)
	}
}

func chatContentToParts(raw interface{}, textType string) interface{} {
	switch c := normalizeChatContent(raw).(type) {
	case string:
		return []map[string]interface{}{{"type": textType, "text": c}}
	case []interface{}:
		parts := make([]map[string]interface{}, 0, len(c))
		for _, p := range c {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			switch t, _ := pm["type"].(string); t {
			case "text":
				txt, _ := pm["text"].(string)
				parts = append(parts, map[string]interface{}{"type": textType, "text": txt})
			case "image_url":
				url := ""
				if u, ok := pm["image_url"].(string); ok {
					url = u
				} else if um, ok := pm["image_url"].(map[string]interface{}); ok {
					url, _ = um["url"].(string)
				}
				if url != "" {
					parts = append(parts, map[string]interface{}{"type": "input_image", "image_url": url})
				}
			}
		}
		if len(parts) > 0 {
			return parts
		}
		return []map[string]interface{}{{"type": textType, "text": ""}}
	default:
		return []map[string]interface{}{{"type": textType, "text": extractStringContent(raw)}}
	}
}
