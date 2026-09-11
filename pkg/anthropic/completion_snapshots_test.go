package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
)

func TestResponsesTerminalSnapshotsAndMultipleTextParts(t *testing.T) {
	tool := `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Bash","arguments":"{\"command\":\"pwd\"}"}`
	for _, stream := range []bool{true, false} {
		for _, tc := range []struct {
			name, sse, text string
			tools           int
		}{
			{"terminalToolOnly", eventsJSON(`{"type":"response.completed","response":{"output":[` + tool + `]}}`), "", 1},
			{"argumentDeltasOnly", eventsJSON(
				`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","call_id":"call_1","type":"function_call","name":"Bash","status":"in_progress","arguments":""}}`,
				`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"command\":"}`,
				`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"pwd\"}"}`,
				`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"command\":\"pwd\"}"}`,
				`{"type":"response.completed"}`,
			), "", 1},
			{"addedStatusIsProvisional", eventsJSON(
				`{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","status":"in_progress"}}`,
				`{"type":"response.output_item.done","item":`+tool+`}`,
				`{"type":"response.completed"}`,
			), "", 1},
			{"multipleParts", eventsJSON(
				`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"A"}`,
				`{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"A"}`,
				`{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":1,"text":"B"}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"A"},{"type":"output_text","text":"B"}]}}`,
				`{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_2","type":"message","content":[{"type":"output_text","text":"C"}]}}`,
				`{"type":"response.completed","response":{"output":[{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"A"},{"type":"output_text","text":"B"}]},{"id":"msg_2","type":"message","content":[{"type":"output_text","text":"C"}]}]}}`,
			), "ABC", 0},
			{"anonymousDeltaThenIdentifiedSnapshot", eventsJSON(
				`{"type":"response.output_text.delta","delta":"Hello"}`,
				`{"type":"response.output_item.done","item":{"type":"message","id":"msg_late","content":[{"type":"output_text","text":"Hello"}]}}`,
				`{"type":"response.completed"}`,
			), "Hello", 0},
			{"emptyCompletion", eventsJSON(`{"type":"response.completed"}`), "", 0},
			{"doubleWrapped", "data: event: response.output_text.done\ndata: data: {\"type\":\"response.output_text.done\",\"text\":\"wrapped\"}\n\ndata: data: {\"type\":\"response.completed\"}\n\n", "wrapped", 0},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				text, tools, stop := completionContent(t, completionFixture(t, "GPT-6 Astra", stream, bodyOf(tc.sse)), stream)
				if text != tc.text || len(tools) != tc.tools {
					t.Fatalf("text=%q tools=%v", text, tools)
				}
				if (tc.tools > 0 && stop != "tool_use") || (tc.tools == 0 && stop != "end_turn") {
					t.Fatalf("stop=%s", stop)
				}
				if tc.tools > 0 {
					args, _ := json.Marshal(tools[0].Input)
					if tools[0].ID != "call_1" || string(args) != `{"command":"pwd"}` {
						t.Fatalf("tool altered: %+v args=%s", tools[0], args)
					}
				}
			})
		}
	}
}

func TestResponsesConflictingSnapshotsFailClosed(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range []struct{ name, sse, reason string }{
			{"textMismatch", eventsJSON(`{"type":"response.output_text.delta","delta":"A"}`, `{"type":"response.output_text.done","text":"B"}`, `{"type":"response.completed"}`), "disagrees"},
			{"argumentsMismatch", eventsJSON(
				`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Bash"}}`,
				`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"a\":"}`,
				`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","arguments":"{\"b\":1}"}}`,
				`{"type":"response.completed"}`,
			), "disagrees"},
			{"nameMismatch", eventsJSON(
				`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","name":"Bash"}}`,
				`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","name":"Read","arguments":"{}"}}`,
				`{"type":"response.completed"}`,
			), "conflicting"},
			{"callIDMismatch", eventsJSON(
				`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Bash"}}`,
				`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_2","arguments":"{}"}}`,
				`{"type":"response.completed"}`,
			), "conflicting"},
			{"terminalError", eventsJSON(`{"type":"response.completed","response":{"error":{"message":"completion with nested error"}}}`), "completion with nested error"},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				assertCompletionFailure(t, completionFixture(t, "GPT-6 Astra", stream, bodyOf(tc.sse)), stream, tc.reason)
			})
		}
	}
}

// Observe the downstream recorder while the upstream is still being read. A
// final post-hoc assertion alone cannot prove tools weren't emitted too early.
type completionGateReader struct {
	prefix   *strings.Reader
	terminal *strings.Reader
	check    func()
	checked  bool
}

func (r *completionGateReader) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		return r.prefix.Read(p)
	}
	if !r.checked {
		r.check()
		r.checked = true
	}
	return r.terminal.Read(p)
}
func (r *completionGateReader) Close() error { return nil }

func TestToolsRemainBufferedUntilTerminalSuccess(t *testing.T) {
	for _, model := range []string{"GPT-6 Astra", "GLM-5.3"} {
		t.Run(model, func(t *testing.T) {
			w := httptest.NewRecorder()
			prefix := eventsJSON(`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Bash","arguments":"{}"}}`)
			terminal := eventsJSON(`{"type":"response.completed"}`)
			if model == "GLM-5.3" {
				prefix = eventsJSON(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Bash","arguments":"{}"}}]}}]}`)
				terminal = eventsJSON(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`, `[DONE]`)
			}
			reader := &completionGateReader{prefix: strings.NewReader(prefix), terminal: strings.NewReader(terminal), check: func() {
				if strings.Contains(w.Body.String(), `"type":"tool_use"`) || strings.Contains(w.Body.String(), "input_json_delta") {
					t.Errorf("tool escaped before response success: %s", w.Body.String())
				}
			}}
			client := joycode.NewClient("test", "test")
			client.SetHTTPClient(&http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: reader}, nil
			})})
			mux := http.NewServeMux()
			NewHandler(client, nil).RegisterRoutes(mux)
			payload := fmt.Sprintf(`{"model":%q,"stream":true,"max_tokens":8192,"messages":[{"role":"user","content":"test"}]}`, model)
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
			_, tools, stop := completionContent(t, w, true)
			if !reader.checked || len(tools) != 1 || stop != "tool_use" {
				t.Fatalf("gate=%t tools=%v stop=%s", reader.checked, tools, stop)
			}
		})
	}
}

func TestChatFinalLineWithoutNewline(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"result\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}"
	text, _, stop := completionContent(t, completionFixture(t, "GLM-5.3", true, bodyOf(sse)), true)
	if text != "result" || stop != "end_turn" {
		t.Fatalf("text=%s stop=%s", text, stop)
	}
}

func TestChatSingleLineWithoutNewline(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"single line"},"finish_reason":"stop"}]}`
	text, _, stop := completionContent(t, completionFixture(t, "GLM-5.3", true, bodyOf(sse)), true)
	if text != "single line" || stop != "end_turn" {
		t.Fatalf("text=%s stop=%s", text, stop)
	}
}

func TestChatSingleLineReadErrorPreserved(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"partial"}}]}`
	w := completionFixture(t, "GLM-5.3", true, func() io.ReadCloser {
		return &errAfterReader{data: []byte(sse), err: io.ErrUnexpectedEOF}
	})
	assertCompletionFailure(t, w, true, "unexpected EOF")
}

func TestResponsesCompletedDoesNotWaitForEOF(t *testing.T) {
	result, err := readResponses(&completionGateReader{
		prefix:   strings.NewReader(eventsJSON(`{"type":"response.output_text.done","text":"finished"}`, `{"type":"response.completed"}`)),
		terminal: strings.NewReader(""),
		check:    func() { t.Error("attempted another upstream read after terminal event") },
	}, nil)
	if err != nil || result.Text != "finished" || result.StopReason != "end_turn" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestReadErrorCauseIsPreserved(t *testing.T) {
	_, err := readResponses(&errAfterReader{data: []byte(eventsJSON(`{"type":"response.output_text.delta","delta":"partial"}`)), err: io.ErrUnexpectedEOF}, nil)
	if err == nil || !strings.Contains(err.Error(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("original error missing: %v", err)
	}
}
