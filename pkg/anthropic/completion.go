package anthropic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

type completionReadError struct{ err error }

func (r completionReadError) Read([]byte) (int, error) { return 0, r.err }

// Tools are buffered until the response has a successful terminal event. Even a
// syntactically complete tool must not escape a subsequently failed response.
type completedTool struct {
	ID        string
	Name      string
	Arguments string
}

func (t *completedTool) validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("upstream tool %q is missing a name", t.ID)
	}
	args := strings.TrimSpace(t.Arguments)
	if !json.Valid([]byte(args)) || !strings.HasPrefix(args, "{") {
		return fmt.Errorf("upstream tool %q (%s) has missing or invalid JSON object arguments", t.ID, t.Name)
	}
	return nil
}

func (t *completedTool) block() ContentBlock {
	id := t.ID
	if id == "" {
		id = "toolu_" + newID()
	}
	return ContentBlock{Type: "tool_use", ID: id, Name: t.Name, Input: json.RawMessage(t.Arguments)}
}

func emitCompletedTool(w http.ResponseWriter, index int, tool completedTool) {
	block := tool.block()
	block.Input = json.RawMessage("{}") // Protocol placeholder, never a replacement for arguments.
	FormatSSE(w, "content_block_start", sseContentBlockStart{Type: "content_block_start", Index: index, ContentBlock: block})
	FormatSSE(w, "content_block_delta", sseContentBlockDelta{
		Type: "content_block_delta", Index: index,
		Delta: deltaText{Type: "input_json_delta", PartialJSON: tool.Arguments},
	})
	FormatSSE(w, "content_block_stop", sseContentBlockStop{Type: "content_block_stop", Index: index})
}

type responseOutputItem struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	CallID    string  `json:"call_id"`
	Name      string  `json:"name"`
	Arguments *string `json:"arguments"`
	Status    string  `json:"status"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type responsesEvent struct {
	Type         string              `json:"type"`
	Delta        string              `json:"delta"`
	Text         string              `json:"text"`
	Arguments    *string             `json:"arguments"`
	Name         string              `json:"name"`
	ItemID       string              `json:"item_id"`
	CallID       string              `json:"call_id"`
	OutputIndex  *int                `json:"output_index"`
	ContentIndex int                 `json:"content_index"`
	Item         *responseOutputItem `json:"item"`
	Part         *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"part"`
	Response *struct {
		Status            string               `json:"status"`
		Usage             *Usage               `json:"usage"`
		Output            []responseOutputItem `json:"output"`
		Error             json.RawMessage      `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	} `json:"response"`
	Usage *Usage          `json:"usage"`
	Error json.RawMessage `json:"error"`
}

type responseText struct {
	text string
	done bool
}

type responseItemAccum struct {
	completedTool
	kind        string
	status      string
	anonymous   bool
	outputIndex *int
	texts       map[int]*responseText
	argsDone    bool
}

type responsesResult struct {
	Text       string
	Tools      []completedTool
	StopReason string
	Usage      Usage
}

// readResponses is shared by streaming and non-streaming handlers. Snapshots
// contain the entire value, not another delta. Item IDs and call IDs are aliases,
// not separate function calls. Keep normal deltas live, but reconcile the final
// item/part order against the emitted prefix before claiming success: an SSE
// consumer cannot retract text when a late snapshot inserts an earlier suffix.
func readResponses(body io.Reader, onText func(string)) (result responsesResult, err error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	byID := make(map[string]*responseItemAccum)
	byIndex := make(map[int]*responseItemAccum)
	var items []*responseItemAccum
	var anonymousText *responseItemAccum
	var emitted strings.Builder
	terminalSnapshot := false
	getItem := func(ev responsesEvent) (*responseItemAccum, error) {
		ids := []string{ev.ItemID, ev.CallID}
		if ev.Item != nil {
			ids = append(ids, ev.Item.ID, ev.Item.CallID)
		}
		var item *responseItemAccum
		for _, id := range ids {
			if id != "" && byID[id] != nil {
				if item != nil && item != byID[id] {
					return nil, fmt.Errorf("conflicting upstream response item IDs")
				}
				item = byID[id]
			}
		}
		index := ev.OutputIndex
		isText := strings.HasPrefix(ev.Type, "response.output_text.") ||
			(ev.Type == "response.content_part.done" && ev.Part != nil && ev.Part.Type == "output_text") ||
			(ev.Item != nil && ev.Item.Type == "message")
		anonymous := isText && index == nil && ev.ItemID == "" && ev.CallID == "" && (ev.Item == nil || (ev.Item.ID == "" && ev.Item.CallID == ""))
		if index != nil && byIndex[*index] != nil {
			if item != nil && item != byIndex[*index] {
				return nil, fmt.Errorf("conflicting upstream response output index %d", *index)
			}
			item = byIndex[*index]
		}
		// Anonymous legacy text has no output index. In particular it must
		// never occupy index zero, which may be a reasoning or function item.
		// Keep the pointer as an alias even after an identifying event arrives.
		if anonymous {
			item = anonymousText
		} else if isText && anonymousText != nil && anonymousText.anonymous {
			if item == nil {
				item = anonymousText
			} else if item != anonymousText {
				// Metadata can precede the anonymous deltas. Join that empty
				// identified accumulator, not two independently received texts.
				if len(item.texts) != 0 || (item.kind != "" && item.kind != "message") {
					return nil, fmt.Errorf("ambiguous upstream anonymous text item")
				}
				item.texts = anonymousText.texts
				for i, candidate := range items {
					if candidate == anonymousText {
						items = append(items[:i], items[i+1:]...)
						break
					}
				}
				anonymousText = item
			}
		}
		if item == nil {
			item = &responseItemAccum{texts: make(map[int]*responseText), anonymous: anonymous}
			items = append(items, item)
		}
		if anonymous {
			anonymousText = item
		} else {
			item.anonymous = false
		}
		if isText {
			if item.kind != "" && item.kind != "message" {
				return nil, fmt.Errorf("conflicting upstream text item type %q", item.kind)
			}
			item.kind = "message"
		}
		for _, id := range ids {
			if id != "" {
				byID[id] = item
			}
		}
		if index != nil {
			if item.outputIndex != nil && *item.outputIndex != *index {
				return nil, fmt.Errorf("conflicting upstream response output index %d", *index)
			}
			i := *index
			item.outputIndex = &i
			byIndex[i] = item
		}
		if ev.CallID != "" {
			if item.ID != "" && item.ID != ev.CallID {
				return nil, fmt.Errorf("conflicting upstream tool call ID for item %q", ev.ItemID)
			}
			item.ID = ev.CallID
		}
		if ev.Name != "" {
			if item.Name != "" && item.Name != ev.Name {
				return nil, fmt.Errorf("conflicting upstream tool name for call %q", item.ID)
			}
			item.Name = ev.Name
		}
		return item, nil
	}
	addText := func(item *responseItemAccum, index int, value string, snapshot bool) error {
		part := item.texts[index]
		if part == nil {
			part = &responseText{}
			item.texts[index] = part
		}
		if snapshot {
			if !strings.HasPrefix(value, part.text) {
				return fmt.Errorf("upstream output_text snapshot disagrees with streamed text")
			}
			value = value[len(part.text):]
			part.done = true
		} else if part.done && value != "" {
			return fmt.Errorf("upstream output_text delta arrived after done")
		}
		part.text += value
		if value != "" && onText != nil && !terminalSnapshot {
			emitted.WriteString(value)
			onText(value)
		}
		return nil
	}
	applyItem := func(ev responsesEvent, snapshot bool) error {
		item, e := getItem(ev)
		if e != nil {
			return e
		}
		src := ev.Item
		if src.Type != "" {
			if item.kind != "" && item.kind != src.Type {
				return fmt.Errorf("conflicting upstream response item type %q", src.Type)
			}
			item.kind = src.Type
		}
		if snapshot {
			// The status on output_item.added is only provisional. Only a
			// final snapshot may veto the successful response terminal event.
			item.status = src.Status
			if item.status == "" {
				item.status = "completed"
			}
		}
		if src.CallID != "" {
			if item.ID != "" && item.ID != src.CallID {
				return fmt.Errorf("conflicting upstream tool call ID for item %q", src.ID)
			}
			item.ID = src.CallID
		}
		if src.Name != "" {
			if item.Name != "" && item.Name != src.Name {
				return fmt.Errorf("conflicting upstream tool name for call %q", item.ID)
			}
			item.Name = src.Name
		}
		if src.Type == "function_call" {
			if src.Arguments != nil && (snapshot || *src.Arguments != "") {
				if !strings.HasPrefix(*src.Arguments, item.Arguments) {
					return fmt.Errorf("upstream tool %q arguments snapshot disagrees with deltas", item.ID)
				}
				item.Arguments = *src.Arguments
				item.argsDone = snapshot
			}
		}
		if snapshot {
			for index, part := range src.Content {
				if part.Type == "output_text" {
					if e := addText(item, index, part.Text, true); e != nil {
						return e
					}
				}
			}
		}
		return nil
	}
	for scanner.Scan() {
		line := unwrapNativeAnthropicSSE(scanner.Text())
		if line == "[DONE]" {
			break // A transport sentinel is not a successful Responses terminal event.
		}
		if line == "" || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:") {
			continue
		}
		var ev responsesEvent
		if e := json.Unmarshal([]byte(line), &ev); e != nil {
			return result, fmt.Errorf("invalid upstream Responses event: %w", e)
		}
		if ev.Usage != nil {
			result.Usage = *ev.Usage
		}
		if ev.Response != nil && ev.Response.Usage != nil {
			result.Usage = *ev.Response.Usage
		}
		if ev.Type == "error" || ev.Type == "response.failed" || (len(ev.Error) > 0 && string(ev.Error) != "null") ||
			(ev.Response != nil && len(ev.Response.Error) > 0 && string(ev.Response.Error) != "null") {
			return result, fmt.Errorf("upstream GPT response error: %s", line)
		}
		switch ev.Type {
		case "response.output_text.delta", "response.output_text.done", "response.content_part.done":
			if ev.Type == "response.content_part.done" && (ev.Part == nil || ev.Part.Type != "output_text") {
				continue // Non-text parts must not claim the anonymous text alias.
			}
			item, e := getItem(ev)
			if e != nil {
				return result, e
			}
			value, snapshot := ev.Delta, false
			if ev.Type == "response.output_text.done" {
				value, snapshot = ev.Text, true
			}
			if ev.Type == "response.content_part.done" {
				value, snapshot = ev.Part.Text, true
			}
			if e := addText(item, ev.ContentIndex, value, snapshot); e != nil {
				return result, e
			}
		case "response.output_item.added", "response.output_item.done":
			if ev.Item == nil {
				return result, fmt.Errorf("upstream %s missing item", ev.Type)
			}
			if e := applyItem(ev, ev.Type == "response.output_item.done"); e != nil {
				return result, e
			}
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			item, e := getItem(ev)
			if e != nil {
				return result, e
			}
			item.kind = "function_call"
			if ev.Type == "response.function_call_arguments.delta" {
				if item.argsDone && ev.Delta != "" {
					return result, fmt.Errorf("upstream tool arguments delta arrived after done")
				}
				item.Arguments += ev.Delta
			} else {
				if ev.Arguments == nil {
					return result, fmt.Errorf("upstream tool arguments.done missing arguments")
				}
				if !strings.HasPrefix(*ev.Arguments, item.Arguments) {
					return result, fmt.Errorf("upstream tool arguments.done disagrees with deltas")
				}
				item.Arguments = *ev.Arguments
				item.argsDone = true
			}
		case "response.completed", "response.incomplete":
			if ev.Type == "response.incomplete" {
				if ev.Response == nil || ev.Response.IncompleteDetails == nil || ev.Response.IncompleteDetails.Reason != "max_output_tokens" {
					return result, fmt.Errorf("upstream GPT response incomplete: %s", line)
				}
				result.StopReason = "max_tokens"
			} else {
				if ev.Response != nil && ev.Response.Status != "" && ev.Response.Status != "completed" {
					return result, fmt.Errorf("upstream response.completed has non-completed status: %s", line)
				}
				result.StopReason = "end_turn"
			}
			// Apply the whole terminal snapshot before emitting any suffix. An
			// earlier item's missing suffix belongs before later items' text.
			terminalSnapshot = true
			if ev.Response != nil {
				for index := range ev.Response.Output {
					if e := applyItem(responsesEvent{Item: &ev.Response.Output[index], OutputIndex: &index}, true); e != nil {
						return result, e
					}
				}
			}
			// Terminal array positions become output indices above. Without a
			// terminal array use event indices; legacy unindexed items follow
			// in arrival order because no stronger ordering evidence exists.
			sort.SliceStable(items, func(i, j int) bool {
				a, b := items[i].outputIndex, items[j].outputIndex
				if a == nil || b == nil {
					return a != nil && b == nil
				}
				return *a < *b
			})
			var text strings.Builder
			for _, item := range items {
				indices := make([]int, 0, len(item.texts))
				for index := range item.texts {
					indices = append(indices, index)
				}
				sort.Ints(indices)
				for _, index := range indices {
					text.WriteString(item.texts[index].text)
				}
			}
			result.Text = text.String()
			if onText != nil {
				if !strings.HasPrefix(result.Text, emitted.String()) {
					return result, fmt.Errorf("upstream output_text order conflicts with already streamed text")
				}
				if suffix := result.Text[emitted.Len():]; suffix != "" {
					onText(suffix)
				}
			}
			// A token-limited response never exposes tools, including complete
			// earlier calls: the model may have been cut off between related calls.
			if result.StopReason == "max_tokens" {
				return result, nil
			}
			seenCalls := make(map[string]bool)
			for _, item := range items {
				if item.kind != "function_call" {
					continue
				}
				if item.status != "" && item.status != "completed" {
					return result, fmt.Errorf("upstream tool %q has unfinished status %q", item.ID, item.status)
				}
				if e := item.validate(); e != nil {
					return result, e
				}
				if item.ID != "" && seenCalls[item.ID] {
					return result, fmt.Errorf("duplicate upstream tool call ID %q", item.ID)
				}
				seenCalls[item.ID] = true
				result.Tools = append(result.Tools, item.completedTool)
			}
			if len(result.Tools) > 0 {
				result.StopReason = "tool_use"
			}
			return result, nil
		}
	}
	if e := scanner.Err(); e != nil {
		return result, fmt.Errorf("reading upstream Responses stream before terminal event: %w", e)
	}
	return result, fmt.Errorf("upstream Responses stream closed (EOF) before response.completed/response.incomplete/response.failed")
}

func chatStopReason(reason string, tools int) (string, error) {
	switch reason {
	case "stop":
		if tools > 0 {
			return "tool_use", nil
		}
		return "end_turn", nil
	case "tool_calls":
		if tools == 0 {
			return "", fmt.Errorf("upstream finish_reason tool_calls without tools")
		}
		return "tool_use", nil
	case "length":
		return "max_tokens", nil
	default:
		return "", fmt.Errorf("upstream Chat completion failed or unfinished: finish_reason=%q", reason)
	}
}

// Validate before TranslateResponse, whose legacy public behavior accepts
// malformed tool JSON. Keep that permissive helper out of the serving path.
func validatedChatResponse(raw map[string]interface{}, model string) (*MessageResponse, error) {
	if raw["error"] != nil {
		payload, _ := json.Marshal(raw["error"])
		return nil, fmt.Errorf("upstream Chat response error: %s", payload)
	}
	choices, _ := raw["choices"].([]interface{})
	if len(choices) == 0 {
		return nil, fmt.Errorf("upstream Chat response missing choices")
	}
	choice, _ := choices[0].(map[string]interface{})
	message, _ := choice["message"].(map[string]interface{})
	if message == nil {
		return nil, fmt.Errorf("upstream Chat response missing message")
	}
	calls, _ := message["tool_calls"].([]interface{})
	reason, _ := choice["finish_reason"].(string)
	stop, err := chatStopReason(reason, len(calls))
	if err != nil {
		return nil, err
	}
	content := []ContentBlock{}
	if text, _ := message["content"].(string); text != "" {
		content = append(content, ContentBlock{Type: "text", Text: text})
	}
	if stop != "max_tokens" {
		seenIDs := make(map[string]bool)
		for _, call := range calls {
			c, _ := call.(map[string]interface{})
			fn, _ := c["function"].(map[string]interface{})
			id, _ := c["id"].(string)
			name, _ := fn["name"].(string)
			args, _ := fn["arguments"].(string)
			tool := completedTool{ID: id, Name: name, Arguments: args}
			if e := tool.validate(); e != nil {
				return nil, e
			}
			if id != "" && seenIDs[id] {
				return nil, fmt.Errorf("duplicate upstream tool call ID %q", id)
			}
			seenIDs[id] = true
			content = append(content, tool.block())
		}
	}
	if len(content) == 0 {
		content = append(content, ContentBlock{Type: "text", Text: ""})
	}
	return &MessageResponse{ID: NewMessageID(), Type: "message", Role: "assistant", Model: model, Content: content, StopReason: &stop, Usage: extractUsage(raw)}, nil
}

func sortedChatTools(calls map[int]*completedTool) ([]completedTool, error) {
	indices := make([]int, 0, len(calls))
	for index := range calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	tools := make([]completedTool, 0, len(calls))
	seenIDs := make(map[string]bool)
	for _, index := range indices {
		tool := calls[index]
		if err := tool.validate(); err != nil {
			return nil, err
		}
		if tool.ID != "" && seenIDs[tool.ID] {
			return nil, fmt.Errorf("duplicate upstream tool call ID %q", tool.ID)
		}
		seenIDs[tool.ID] = true
		tools = append(tools, *tool)
	}
	return tools, nil
}
