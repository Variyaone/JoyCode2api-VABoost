package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
	"github.com/vibe-coding-labs/JoyCode2Api/pkg/store"
)

const budgetNativeResponse = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_budget\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

type budgetRoundTripper func(*http.Request) (*http.Response, error)

func (f budgetRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativeThinkingBudgetAfterMaxTokensCap(t *testing.T) {
	for _, route := range []struct{ name, model, account, global string }{
		{"native", "Claude-Opus-5", "", ""},
		{"native 4.8", "Claude-Opus-4.8", "", ""},
		{"account alias", "desktop-alias", "Claude-Opus-5", "GPT-6 Astra"},
		{"global alias", "desktop-alias", "", "Claude-Opus-5"},
	} {
		t.Run(route.name, func(t *testing.T) {
			var db *store.Store
			if route.global != "" {
				var err error
				db, err = store.Open(filepath.Join(t.TempDir(), "budget.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
				if err := db.SetSetting("default_model", route.global); err != nil {
					t.Fatal(err)
				}
			}
			for _, tc := range []struct {
				name, thinking       string
				maxTokens, effective int
				budget               int
				reject               bool
			}{
				{"below cap", `{"type":"enabled","budget_tokens":32767}`, 64000, 32768, 32767, false},
				{"equal cap", `{"type":"enabled","budget_tokens":32768}`, 64000, 32768, 32768, true},
				{"above cap", `{"type":"enabled","budget_tokens":40000}`, 64000, 32768, 40000, true},
				{"equal uncapped max", `{"type":"enabled","budget_tokens":8192}`, 8192, 8192, 8192, true},
				{"above default", `{"type":"enabled","budget_tokens":10000}`, 0, 8192, 10000, true},
				{"legal enabled", `{"type":"enabled","budget_tokens":4096}`, 8192, 8192, 4096, false},
				{"legal adaptive", `{"type":"adaptive","display":"summarized"}`, 64000, 32768, 0, false},
				{"disabled", `{"type":"disabled"}`, 64000, 32768, 0, false},
			} {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
						calls := 0
						client := joycode.NewClient("synthetic", "synthetic")
						client.SetHTTPClient(&http.Client{Transport: budgetRoundTripper(func(r *http.Request) (*http.Response, error) {
							calls++
							var body map[string]interface{}
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Fatal(err)
							}
							if tc.reject {
								t.Errorf("invalid budget reached upstream: %v", body)
							}
							if r.URL.Query().Get("functionId") != "anthropic_completions" || body["max_tokens"] != float64(tc.effective) {
								t.Errorf("unexpected route/max_tokens: %s %v", r.URL, body)
							}
							thinking, _ := body["thinking"].(map[string]interface{})
							var wantThinking map[string]interface{}
							if err := json.Unmarshal([]byte(tc.thinking), &wantThinking); err != nil {
								t.Fatal(err)
							}
							gotJSON, _ := json.Marshal(thinking)
							wantJSON, _ := json.Marshal(wantThinking)
							if string(gotJSON) != string(wantJSON) {
								t.Errorf("thinking was silently changed: got %s want %s", gotJSON, wantJSON)
							}
							return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(budgetNativeResponse))}, nil
						})})
						mux := http.NewServeMux()
						NewHandler(client, db).RegisterRoutes(mux)
						payload := fmt.Sprintf(`{"model":%q,"stream":%t,"max_tokens":%d,"thinking":%s,"output_config":{"effort":"max"},"messages":[{"role":"user","content":"test"}]}`, route.model, stream, tc.maxTokens, tc.thinking)
						r := store.InitAccountModel(httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
						store.SetAccountDefaultModel(r, route.account)
						w := httptest.NewRecorder()
						mux.ServeHTTP(w, r)
						if tc.reject {
							if calls != 0 || w.Code != http.StatusBadRequest {
								t.Fatalf("calls=%d status=%d response=%s", calls, w.Code, w.Body.String())
							}
							var response struct {
								Type  string                         `json:"type"`
								Error struct{ Type, Message string } `json:"error"`
							}
							if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Type != "error" || response.Error.Type != "invalid_request_error" {
								t.Fatalf("expected non-retryable JSON error before SSE headers: %s (err=%v)", w.Body.String(), err)
							}
							for _, detail := range []string{"budget_tokens", "max_tokens", fmt.Sprint(tc.budget), fmt.Sprint(tc.effective)} {
								if !strings.Contains(response.Error.Message, detail) {
									t.Errorf("error missing %q: %s", detail, response.Error.Message)
								}
							}
						} else if calls != 1 || w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"ok"`) {
							t.Fatalf("calls=%d status=%d response=%s", calls, w.Code, w.Body.String())
						}
					})
				}
			}
		})
	}
}

func TestNativeBudgetValidationDoesNotApplyToChatTranslation(t *testing.T) {
	for _, model := range []string{"Claude-Opus-5", "MiniMax-M3"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				db, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
				if model == "Claude-Opus-5" {
					if err := db.SetSetting("enable_claude", "false"); err != nil {
						t.Fatal(err)
					}
				}
				calls := 0
				client := joycode.NewClient("synthetic", "synthetic")
				client.SetHTTPClient(&http.Client{Transport: budgetRoundTripper(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Query().Get("functionId") != "chat_completions" {
						t.Errorf("unexpected native route: %s", r.URL)
					}
					response := `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`
					if stream {
						response = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
				})})
				mux := http.NewServeMux()
				NewHandler(client, db).RegisterRoutes(mux)
				payload := fmt.Sprintf(`{"model":%q,"stream":%t,"max_tokens":64000,"thinking":{"type":"enabled","budget_tokens":40000},"messages":[{"role":"user","content":"test"}]}`, model, stream)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
				if calls != 1 || w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"ok"`) {
					t.Fatalf("native-only validation affected chat: calls=%d status=%d response=%s", calls, w.Code, w.Body.String())
				}
			})
		}
	}
}
