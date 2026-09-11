package anthropic

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompletionOnlyModelDoesNotFallBack(t *testing.T) {
	for _, stream := range []string{"true", "false"} {
		mux := http.NewServeMux()
		NewHandler(nil, nil).RegisterRoutes(mux)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"JoyCode-Base-V3","stream":`+stream+`,"messages":[{"role":"user","content":"hello"}]}`)))
		if w.Code != 400 || !strings.Contains(w.Body.String(), "JoyCode-Base-V3") {
			t.Fatalf("expected explicit unsupported, got %d %s", w.Code, w.Body.String())
		}
	}
}
