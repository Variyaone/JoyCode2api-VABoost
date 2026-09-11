package anthropic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
)

func runResponsesFixture(t *testing.T, stream string) string {
	t.Helper()
	client := joycode.NewClient("test", "test")
	client.SetHTTPClient(&http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(stream))}, nil
	})})
	mux := http.NewServeMux()
	NewHandler(client, nil).RegisterRoutes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"GPT-6 Astra","stream":true,"max_tokens":8192,"messages":[{"role":"user","content":"test"}]}`)))
	return w.Body.String()
}

func TestResponsesDoesNotAppendEmptyTextAfterFinalText(t *testing.T) {
	out := runResponsesFixture(t, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"actual result\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":2,\"output_tokens\":3}}}\n\n")
	if count := strings.Count(out, "event: content_block_start"); count != 1 {
		t.Fatalf("got %d content blocks; trailing empty block can replace SDK final result:\n%s", count, out)
	}
}

func TestResponsesPrematureEOFMustNotClaimEndTurn(t *testing.T) {
	out := runResponsesFixture(t, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	if !strings.Contains(out, "event: error") || strings.Contains(out, "event: message_stop") {
		t.Fatalf("premature EOF falsely completed:\n%s", out)
	}
}

func TestResponsesInvalidToolArgumentsMustNotExecuteEmptyObject(t *testing.T) {
	out := runResponsesFixture(t, "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"fc_1\",\"call_id\":\"call_1\",\"type\":\"function_call\",\"name\":\"Bash\"}}\n\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"fc_1\",\"call_id\":\"call_1\",\"type\":\"function_call\",\"name\":\"Bash\",\"arguments\":\"{\\\"command\\\":\"}}\n\ndata: {\"type\":\"response.completed\"}\n\n")
	if !strings.Contains(out, "event: error") || strings.Contains(out, `"stop_reason":"tool_use"`) {
		t.Fatalf("malformed tool arguments treated as executable:\n%s", out)
	}
}
