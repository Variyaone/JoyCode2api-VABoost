package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTruncationKeepsSeparatedToolPair(t *testing.T) {
	// Mid-turn system messages mean a tool result need not be immediately
	// adjacent to its assistant call, nor at an even message index.
	req := &MessageRequest{Messages: []MessageParam{
		{Role: "user", Content: json.RawMessage(`"initial"`)},
		{Role: "assistant", Content: json.RawMessage(`"old answer"`)},
		{Role: "user", Content: json.RawMessage(`"old followup"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_keep","name":"probe","input":{}}]`)},
		{Role: "system", Content: json.RawMessage(`"queued instruction"`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_keep","content":"important result"}]`)},
		{Role: "assistant", Content: json.RawMessage(`"continued"`)},
		{Role: "user", Content: json.RawMessage(`"latest"`)},
		{Role: "assistant", Content: json.RawMessage(`"working"`)},
		{Role: "user", Content: json.RawMessage(`"newest"`)},
	}}
	if !truncateMessages(req) {
		t.Fatal("expected some truncation")
	}
	raw, _ := json.Marshal(req.Messages)
	if !strings.Contains(string(raw), `"id":"call_keep"`) || !strings.Contains(string(raw), `"tool_use_id":"call_keep"`) {
		t.Fatalf("truncation split tool pair: %s", raw)
	}
}
