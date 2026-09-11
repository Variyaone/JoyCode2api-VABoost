package anthropic

import (
	"fmt"
	"strings"
	"testing"
)

// These six public-handler cases reproduce the three ordering regressions in
// both modes. Streaming cannot retract AC once a terminal snapshot reveals ABC.
func TestResponsesSnapshotOrderBothModes(t *testing.T) {
	first := `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"First","arguments":"{\"a\":1}"}`
	second := `{"type":"function_call","id":"fc_2","call_id":"call_2","name":"Second","arguments":"{\"b\":2}"}`
	for _, stream := range []bool{true, false} {
		for _, tc := range []struct {
			name, sse, text string
			tools           int
			streamError     bool
		}{
			{"anonymousAfterReasoning", eventsJSON(
				`{"type":"response.output_text.delta","delta":"Hello"}`,
				`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1"},{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"Hello"}]}]}}`,
			), "Hello", 0, false},
			{"earlierItemMissingSuffix", eventsJSON(
				`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"delta":"A"}`,
				`{"type":"response.output_text.delta","item_id":"msg_2","output_index":1,"delta":"C"}`,
				`{"type":"response.completed","response":{"output":[{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"AB"}]},{"type":"message","id":"msg_2","content":[{"type":"output_text","text":"C"}]}]}}`,
			), "ABC", 0, true},
			{"toolsFollowTerminalOrder", eventsJSON(
				`{"type":"response.output_item.done","output_index":1,"item":`+second+`}`,
				`{"type":"response.completed","response":{"output":[`+first+`,`+second+`]}}`,
			), "", 2, false},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				w := completionFixture(t, "GPT-6 Astra", stream, bodyOf(tc.sse))
				if stream && tc.streamError {
					assertCompletionFailure(t, w, true, "order")
					return
				}
				text, tools, stop := completionContent(t, w, stream)
				if text != tc.text || len(tools) != tc.tools {
					t.Fatalf("text=%q tools=%+v", text, tools)
				}
				if tc.tools == 0 && stop != "end_turn" || tc.tools > 0 && stop != "tool_use" {
					t.Fatalf("stop=%s", stop)
				}
				for i, tool := range tools {
					if tool.ID != fmt.Sprintf("call_%d", i+1) || tool.Name != []string{"First", "Second"}[i] {
						t.Fatalf("tools not in output order: %+v", tools)
					}
				}
			})
		}
	}
}

func TestResponsesSnapshotOrderEdgeCases(t *testing.T) {
	for _, stream := range []bool{true, false} {
		for _, tc := range []struct {
			name, sse, text string
			streamError     bool
		}{
			{"anonymousGetsIndexAndSuffix", eventsJSON(
				`{"type":"response.output_text.delta","delta":"Hello"}`,
				`{"type":"response.output_text.done","item_id":"msg_1","output_index":1,"text":"Hello world"}`,
				`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1"},{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"Hello world"}]}]}}`,
			), "Hello world", false},
			{"metadataPrecedesAnonymousText", eventsJSON(
				`{"type":"response.output_item.added","output_index":1,"item":{"id":"msg_1","type":"message","content":[]}}`,
				`{"type":"response.output_text.delta","delta":"Hello"}`,
				`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1"},{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"Hello!"}]}]}}`,
			), "Hello!", false},
			{"anonymousAliasSurvivesIdentification", eventsJSON(
				`{"type":"response.output_text.delta","delta":"A"}`,
				`{"type":"response.output_item.added","output_index":1,"item":{"id":"msg_1","type":"message"}}`,
				`{"type":"response.output_text.delta","delta":"B"}`,
				`{"type":"response.output_text.done","item_id":"msg_1","content_index":0,"text":"AB"}`,
				`{"type":"response.content_part.done","content_index":0,"part":{"type":"output_text","text":"AB"}}`,
				`{"type":"response.output_text.done","item_id":"msg_1","content_index":1,"text":"C"}`,
				`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1"},{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"AB"},{"type":"output_text","text":"C!"}]}]}}`,
			), "ABC!", false},
			{"earlierPartMissingSuffix", eventsJSON(
				`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"A"}`,
				`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":1,"delta":"C"}`,
				`{"type":"response.completed","response":{"output":[{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"AB"},{"type":"output_text","text":"C"}]}]}}`,
			), "ABC", true},
			{"partsArriveBackwards", eventsJSON(
				`{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":1,"text":"B"}`,
				`{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"A"}`,
				`{"type":"response.completed"}`,
			), "AB", true},
			{"terminalOnlyPartsAndItems", eventsJSON(
				`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1"},{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"A"},{"type":"output_text","text":"B"}]},{"type":"message","id":"msg_2","content":[{"type":"output_text","text":"C"}]}]}}`,
			), "ABC", false},
			{"nonTextPartDoesNotClaimAnonymous", eventsJSON(
				`{"type":"response.output_text.delta","delta":"Hello"}`,
				`{"type":"response.content_part.done","item_id":"rs_1","output_index":0,"part":{"type":"summary_text","text":"reasoning"}}`,
				`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1"},{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"Hello"}]}]}}`,
			), "Hello", false},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				w := completionFixture(t, "GPT-6 Astra", stream, bodyOf(tc.sse))
				if stream && tc.streamError {
					assertCompletionFailure(t, w, true, "order")
					return
				}
				text, tools, stop := completionContent(t, w, stream)
				if text != tc.text || len(tools) != 0 || stop != "end_turn" {
					t.Fatalf("text=%q tools=%+v stop=%s", text, tools, stop)
				}
			})
		}
	}
}

func TestResponsesSnapshotOrderToolSafety(t *testing.T) {
	first := `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"First","arguments":"{}"}`
	second := `{"type":"function_call","id":"fc_2","call_id":"call_2","name":"Second","arguments":"{}"}`
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprintf("eventIndicesWithoutTerminalOutput/stream=%t", stream), func(t *testing.T) {
			sse := eventsJSON(
				`{"type":"response.output_item.done","output_index":1,"item":`+second+`}`,
				`{"type":"response.output_item.done","output_index":0,"item":`+first+`}`,
				`{"type":"response.completed"}`,
			)
			_, tools, stop := completionContent(t, completionFixture(t, "GPT-6 Astra", stream, bodyOf(sse)), stream)
			if len(tools) != 2 || tools[0].ID != "call_1" || tools[1].ID != "call_2" || stop != "tool_use" {
				t.Fatalf("tools=%+v stop=%s", tools, stop)
			}
		})
		t.Run(fmt.Sprintf("incompleteSuppressesReorderedTools/stream=%t", stream), func(t *testing.T) {
			sse := eventsJSON(
				`{"type":"response.output_item.done","output_index":1,"item":`+second+`}`,
				`{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"},"output":[`+first+`,`+second+`]}}`,
			)
			text, tools, stop := completionContent(t, completionFixture(t, "GPT-6 Astra", stream, bodyOf(sse)), stream)
			if text != "" || len(tools) != 0 || stop != "max_tokens" {
				t.Fatalf("text=%q tools=%+v stop=%s", text, tools, stop)
			}
		})
	}
	t.Run("orderErrorWithholdsCompletedTools", func(t *testing.T) {
		sse := eventsJSON(
			`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"delta":"A"}`,
			`{"type":"response.output_text.delta","item_id":"msg_2","output_index":1,"delta":"C"}`,
			`{"type":"response.output_item.done","output_index":2,"item":`+first+`}`,
			`{"type":"response.completed","response":{"output":[{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"AB"}]},{"type":"message","id":"msg_2","content":[{"type":"output_text","text":"C"}]},`+first+`]}}`,
		)
		assertCompletionFailure(t, completionFixture(t, "GPT-6 Astra", true, bodyOf(sse)), true, "order")
	})
}

func TestResponsesSnapshotOrderKeepsLiveDeltas(t *testing.T) {
	var emitted strings.Builder
	reader := &completionGateReader{
		prefix: strings.NewReader(eventsJSON(
			`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"delta":"Hello"}`,
		)),
		terminal: strings.NewReader(eventsJSON(
			`{"type":"response.completed","response":{"output":[{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"Hello world"}]}]}}`,
		)),
		check: func() {
			if emitted.String() != "Hello" {
				t.Errorf("normal delta not delivered before terminal: %q", emitted.String())
			}
		},
	}
	result, err := readResponses(reader, func(s string) { emitted.WriteString(s) })
	if err != nil || !reader.checked || result.Text != "Hello world" || emitted.String() != result.Text {
		t.Fatalf("gate=%t result=%+v emitted=%q err=%v", reader.checked, result, emitted.String(), err)
	}
}
