package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
)

func completionFixture(t *testing.T, model string, stream bool, body func() io.ReadCloser) *httptest.ResponseRecorder {
	t.Helper()
	client := joycode.NewClient("test", "test")
	client.SetHTTPClient(&http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body()}, nil
	})})
	mux := http.NewServeMux()
	NewHandler(client, nil).RegisterRoutes(mux)
	w := httptest.NewRecorder()
	payload := fmt.Sprintf(`{"model":%q,"stream":%t,"max_tokens":8192,"messages":[{"role":"user","content":"test"}]}`, model, stream)
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
	return w
}

func eventsJSON(events ...string) string {
	return "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
}

// Parse the public response rather than merely matching substrings. Also check
// contiguous indices and exactly one stop per started content block.
func completionContent(t *testing.T, w *httptest.ResponseRecorder, stream bool) (string, []ContentBlock, string) {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("unexpected HTTP %d: %s", w.Code, w.Body.String())
	}
	var blocks []ContentBlock
	stop := ""
	if !stream {
		var response MessageResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		blocks = response.Content
		if response.StopReason == nil {
			t.Fatalf("missing stop_reason: %s", w.Body.String())
		}
		stop = *response.StopReason
	} else {
		args := make(map[int]string)
		closed := make(map[int]bool)
		messageStops := 0
		for _, line := range strings.Split(w.Body.String(), "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var ev struct {
				Type         string       `json:"type"`
				Index        int          `json:"index"`
				ContentBlock ContentBlock `json:"content_block"`
				Delta        struct {
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
					StopReason  string `json:"stop_reason"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev); err != nil {
				t.Fatal(err)
			}
			switch ev.Type {
			case "error":
				t.Fatalf("unexpected stream error: %s", w.Body.String())
			case "content_block_start":
				if ev.Index != len(blocks) {
					t.Fatalf("noncontiguous block %d: %s", ev.Index, w.Body.String())
				}
				blocks = append(blocks, ev.ContentBlock)
			case "content_block_delta":
				if ev.Index >= len(blocks) || closed[ev.Index] {
					t.Fatal("delta without open block")
				}
				blocks[ev.Index].Text += ev.Delta.Text
				args[ev.Index] += ev.Delta.PartialJSON
			case "content_block_stop":
				if ev.Index >= len(blocks) || closed[ev.Index] {
					t.Fatal("duplicate stop or stop without start")
				}
				closed[ev.Index] = true
			case "message_delta":
				stop = ev.Delta.StopReason
			case "message_stop":
				messageStops++
			}
		}
		if messageStops != 1 || len(closed) != len(blocks) {
			t.Fatalf("unfinished blocks/message: %s", w.Body.String())
		}
		for i := range blocks {
			if blocks[i].Type == "tool_use" {
				blocks[i].Input = json.RawMessage(args[i])
			}
		}
	}
	var text strings.Builder
	var tools []ContentBlock
	for _, block := range blocks {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
		if block.Type == "tool_use" {
			tools = append(tools, block)
		}
	}
	return text.String(), tools, stop
}

func assertCompletionFailure(t *testing.T, w *httptest.ResponseRecorder, stream bool, reason string) {
	t.Helper()
	out := w.Body.String()
	if stream {
		if strings.Count(out, "event: error") != 1 || strings.Contains(out, "event: message_stop") || strings.Contains(out, `"type":"tool_use"`) || strings.Contains(out, "input_json_delta") {
			t.Fatalf("error must not expose tools or claim completion: %s", out)
		}
	} else if w.Code < 400 || !strings.Contains(out, `"type":"error"`) {
		t.Fatalf("unexpected success HTTP %d: %s", w.Code, out)
	}
	if reason != "" && !strings.Contains(out, reason) {
		t.Fatalf("original reason %q lost: %s", reason, out)
	}
}

func TestResponsesCompletionFailuresBothModes(t *testing.T) {
	tool := `{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Bash","arguments":"{}"}}`
	for _, stream := range []bool{true, false} {
		for _, tc := range []struct {
			name, sse, reason string
			readErr           error
		}{
			{"empty", "", "EOF", nil},
			{"textEOF", eventsJSON(`{"type":"response.output_text.delta","delta":"partial"}`), "EOF", nil},
			{"toolEOF", eventsJSON(tool), "EOF", nil},
			{"toolDoneSentinel", eventsJSON(tool, `[DONE]`), "EOF", nil},
			{"readError", eventsJSON(tool), "connection reset fixture", errors.New("connection reset fixture")},
			{"scannerOverflow", eventsJSON(tool) + "data: " + strings.Repeat("x", 1024*1024), "token too long", nil},
			{"failedAfterTool", eventsJSON(tool, `{"type":"response.failed","response":{"error":{"message":"specific failure fixture"}}}`), "specific failure fixture", nil},
			{"errorAfterTool", eventsJSON(tool, `{"type":"error","message":"specific error fixture"}`), "specific error fixture", nil},
			{"incompleteFilter", eventsJSON(tool, `{"type":"response.incomplete","response":{"incomplete_details":{"reason":"content_filter"}}}`), "content_filter", nil},
			{"incompleteUnknown", eventsJSON(tool, `{"type":"response.incomplete"}`), "incomplete", nil},
			{"badStatus", eventsJSON(tool, `{"type":"response.completed","response":{"status":"failed"}}`), "failed", nil},
			{"malformedEvent", eventsJSON(tool, `{"type":`), "invalid upstream", nil},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				w := completionFixture(t, "GPT-6 Astra", stream, func() io.ReadCloser {
					if tc.readErr != nil {
						return &errAfterReader{data: []byte(tc.sse), err: tc.readErr}
					}
					return io.NopCloser(strings.NewReader(tc.sse))
				})
				assertCompletionFailure(t, w, stream, tc.reason)
			})
		}
	}
}

func TestResponsesToolValidationBothModes(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range []struct{ name, tool string }{
			{"missingArguments", `"name":"Bash"`},
			{"emptyArguments", `"name":"Bash","arguments":""`},
			{"invalidArguments", `"name":"Bash","arguments":"{\"command\":"`},
			{"nullArguments", `"name":"Bash","arguments":"null"`},
			{"arrayArguments", `"name":"Bash","arguments":"[]"`},
			{"missingName", `"arguments":"{}"`},
			{"blankName", `"name":" ","arguments":"{}"`},
			{"incompleteStatus", `"name":"Bash","arguments":"{}","status":"incomplete"`},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				// A good first tool must also be withheld when another tool fails.
				sse := eventsJSON(
					`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_ok","call_id":"call_ok","name":"Good","arguments":"{}"}}`,
					`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_bad","call_id":"call_bad",`+tc.tool+`}}`,
					`{"type":"response.completed"}`,
				)
				assertCompletionFailure(t, completionFixture(t, "GPT-6 Astra", stream, bodyOf(sse)), stream, "upstream tool")
			})
		}
	}
}

func TestResponsesTextSnapshotsBothModes(t *testing.T) {
	item := `{"type":"message","id":"msg_text","content":[{"type":"output_text","text":"Hello world"}]}`
	for _, stream := range []bool{true, false} {
		for _, tc := range []struct {
			name   string
			events []string
		}{
			{"textDoneOnly", []string{`{"type":"response.output_text.done","item_id":"msg_text","output_index":0,"text":"Hello world"}`}},
			{"itemDoneOnly", []string{`{"type":"response.output_item.done","output_index":0,"item":` + item + `}`}},
			{"completedOnly", nil},
			{"allSnapshots", []string{
				`{"type":"response.output_text.delta","item_id":"msg_text","output_index":0,"delta":"Hello "}`,
				`{"type":"response.output_text.delta","item_id":"msg_text","output_index":0,"delta":"world"}`,
				`{"type":"response.output_text.done","item_id":"msg_text","output_index":0,"text":"Hello world"}`,
				`{"type":"response.content_part.done","item_id":"msg_text","output_index":0,"part":{"type":"output_text","text":"Hello world"}}`,
				`{"type":"response.output_item.done","output_index":0,"item":` + item + `}`,
			}},
			{"missingDeltaSuffix", []string{`{"type":"response.output_text.delta","item_id":"msg_text","output_index":0,"delta":"Hello"}`}},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				events := append(tc.events, `{"type":"response.completed","response":{"output":[`+item+`],"usage":{"input_tokens":10,"output_tokens":20}}}`)
				w := completionFixture(t, "GPT-6 Astra", stream, bodyOf(eventsJSON(events...)))
				text, tools, stop := completionContent(t, w, stream)
				if text != "Hello world" || len(tools) != 0 || stop != "end_turn" {
					t.Fatalf("text=%q tools=%v stop=%s", text, tools, stop)
				}
				if !strings.Contains(w.Body.String(), `"output_tokens":20`) {
					t.Fatal("usage lost")
				}
				if stream && strings.Count(w.Body.String(), "event: content_block_start") != 1 {
					t.Fatal("extra empty text block")
				}
			})
		}
	}
}

func TestResponsesToolAliasesAndSnapshotsBothModes(t *testing.T) {
	first := `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"First","arguments":"{\"a\":1}"}`
	second := `{"type":"function_call","id":"fc_2","call_id":"call_2","name":"Second","arguments":"{\"b\":2}"}`
	sse := eventsJSON(
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"{\"a\":"}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"Second"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"call_2","delta":"{\"b\":2}"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"1}"}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"a\":1}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":`+first+`}`,
		`{"type":"response.output_item.done","output_index":1,"item":`+second+`}`,
		`{"type":"response.output_item.done","output_index":0,"item":`+first+`}`,
		`{"type":"response.completed","response":{"output":[`+first+`,`+second+`]}}`,
	)
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			text, tools, stop := completionContent(t, completionFixture(t, "GPT-6 Astra", stream, bodyOf(sse)), stream)
			if text != "" || len(tools) != 2 || stop != "tool_use" {
				t.Fatalf("text=%q tools=%v stop=%s", text, tools, stop)
			}
			for i, tool := range tools {
				if tool.ID != fmt.Sprintf("call_%d", i+1) || tool.Name != []string{"First", "Second"}[i] {
					t.Fatalf("wrong tool identity: %+v", tool)
				}
				args, _ := json.Marshal(tool.Input)
				if string(args) != []string{`{"a":1}`, `{"b":2}`}[i] {
					t.Fatalf("duplicated/lost arguments: %s", args)
				}
			}
		})
	}
}

func TestResponsesMaxTokensSuppressesAllTools(t *testing.T) {
	sse := eventsJSON(
		`{"type":"response.output_text.delta","delta":"partial result"}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_good","name":"First","arguments":"{}"}}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_bad","name":"Second","arguments":"{\"a\":"}}`,
		`{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":2,"output_tokens":8192}}}`,
	)
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			text, tools, stop := completionContent(t, completionFixture(t, "GPT-6 Astra", stream, bodyOf(sse)), stream)
			if text != "partial result" || len(tools) != 0 || stop != "max_tokens" {
				t.Fatalf("text=%q tools=%v stop=%s", text, tools, stop)
			}
		})
	}
}

func TestChatSparseIndicesLateMetadata(t *testing.T) {
	sse := eventsJSON(
		`{"choices":[{"delta":{"tool_calls":[{"index":7,"function":{"arguments":"{\"b\":"}},{"index":2,"function":{"arguments":"{"}}]}}]}`,
		`{"choices":[{"delta":{"content":"before tools","tool_calls":[{"index":2,"id":"call_2","function":{"name":"First","arguments":"\"a\":1}"}},{"index":7,"id":"call_7","function":{"name":"Second","arguments":"2}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":22}}`,
		`[DONE]`,
	)
	w := completionFixture(t, "GLM-5.3", true, bodyOf(sse))
	text, tools, stop := completionContent(t, w, true)
	if text != "before tools" || len(tools) != 2 || stop != "tool_use" {
		t.Fatalf("text=%q tools=%v stop=%s", text, tools, stop)
	}
	if tools[0].ID != "call_2" || tools[0].Name != "First" || tools[1].ID != "call_7" || tools[1].Name != "Second" {
		t.Fatalf("lost late metadata: %+v", tools)
	}
	for i, tool := range tools {
		args, _ := json.Marshal(tool.Input)
		if string(args) != []string{`{"a":1}`, `{"b":2}`}[i] {
			t.Fatalf("wrong arguments: %s", args)
		}
	}
	if !strings.Contains(w.Body.String(), `"output_tokens":22`) {
		t.Fatal("usage trailer lost")
	}
}

func TestChatNoToolEmissionOnFailure(t *testing.T) {
	good := `{"choices":[{"delta":{"tool_calls":[{"index":3,"id":"call_good","function":{"name":"Good","arguments":"{}"}}]}}]}`
	for _, tc := range []struct {
		name, ending, reason string
		readErr              error
	}{
		{"eof", "", "EOF", nil},
		{"doneWithoutFinish", `data: [DONE]` + "\n", "finish_reason", nil},
		{"malformedArgs", eventsJSON(`{"choices":[{"delta":{"tool_calls":[{"index":7,"id":"call_bad","function":{"name":"Bad","arguments":"{"}}]},"finish_reason":"tool_calls"}]}`), "invalid JSON", nil},
		{"missingArgs", eventsJSON(`{"choices":[{"delta":{"tool_calls":[{"index":7,"id":"call_bad","function":{"name":"Bad"}}]},"finish_reason":"tool_calls"}]}`), "invalid JSON", nil},
		{"missingName", eventsJSON(`{"choices":[{"delta":{"tool_calls":[{"index":7,"function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`), "missing a name", nil},
		{"filter", eventsJSON(`{"choices":[{"delta":{},"finish_reason":"content_filter"}]}`), "content_filter", nil},
		{"upstreamError", eventsJSON(`{"error":{"message":"original chat failure"}}`), "original chat failure", nil},
		{"errorAfterFinish", eventsJSON(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`, `{"error":{"message":"trailer failure"}}`), "trailer failure", nil},
		{"unknownFinish", eventsJSON(`{"choices":[{"delta":{},"finish_reason":"mystery"}]}`), "mystery", nil},
		{"readError", "", "chat connection reset fixture", errors.New("chat connection reset fixture")},
		{"readErrorAfterFinish", eventsJSON(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`), "chat trailer read fixture", errors.New("chat trailer read fixture")},
		{"errorNoFinalNewline", `data: {"error":{"message":"unterminated line failure"}}`, "unterminated line failure", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sse := eventsJSON(good) + tc.ending
			w := completionFixture(t, "GLM-5.3", true, func() io.ReadCloser {
				if tc.readErr != nil {
					return &errAfterReader{data: []byte(sse), err: tc.readErr}
				}
				return io.NopCloser(strings.NewReader(sse))
			})
			assertCompletionFailure(t, w, true, tc.reason)
		})
	}
}

func TestChatLengthSuppressesAllTools(t *testing.T) {
	sse := eventsJSON(
		`{"choices":[{"delta":{"content":"partial","tool_calls":[{"index":5,"id":"call_a","function":{"name":"Bad","arguments":"{"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"length"}]}`,
	)
	text, tools, stop := completionContent(t, completionFixture(t, "GLM-5.3", true, bodyOf(sse)), true)
	if text != "partial" || len(tools) != 0 || stop != "max_tokens" {
		t.Fatalf("text=%q tools=%v stop=%s", text, tools, stop)
	}
}

func TestChatNonStreamValidation(t *testing.T) {
	for _, tc := range []struct {
		name, payload, wantStop, wantText, reason string
		wantTools                                 int
	}{
		{"missingChoices", `{}`, "", "", "missing choices", 0},
		{"missingFinish", `{"choices":[{"message":{"content":"partial"}}]}`, "", "", "finish_reason", 0},
		{"badTool", `{"choices":[{"message":{"tool_calls":[{"id":"a","function":{"name":"Bash","arguments":"{"}}]},"finish_reason":"tool_calls"}]}`, "", "", "invalid JSON", 0},
		{"missingName", `{"choices":[{"message":{"tool_calls":[{"id":"a","function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, "", "", "missing a name", 0},
		{"missingArguments", `{"choices":[{"message":{"tool_calls":[{"id":"a","function":{"name":"Bash"}}]},"finish_reason":"tool_calls"}]}`, "", "", "invalid JSON", 0},
		{"length", `{"choices":[{"message":{"content":"partial","tool_calls":[{"id":"a","function":{"name":"Bash","arguments":"{"}}]},"finish_reason":"length"}]}`, "max_tokens", "partial", "", 0},
		{"validTextAndTool", `{"choices":[{"message":{"content":"explanation","tool_calls":[{"id":"a","function":{"name":"Bash","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, "tool_use", "explanation", "", 1},
		{"validText", `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`, "end_turn", "done", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := completionFixture(t, "GLM-5.3", false, bodyOf(tc.payload))
			if tc.reason != "" {
				assertCompletionFailure(t, w, false, tc.reason)
				return
			}
			text, tools, stop := completionContent(t, w, false)
			if text != tc.wantText || len(tools) != tc.wantTools || stop != tc.wantStop {
				t.Fatalf("text=%q tools=%v stop=%s", text, tools, stop)
			}
		})
	}
}
