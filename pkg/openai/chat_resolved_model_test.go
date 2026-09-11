package openai

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

type resolvedModelRoundTripper func(*http.Request) (*http.Response, error)

func (f resolvedModelRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Verify the actual serialized request, not just ResolveModel's return value.
// All transports are in-memory; defaults live only in a temporary database.
func TestChatResolvedModelOutboundBody(t *testing.T) {
	for _, tc := range []struct {
		name, model, account, global, want string
	}{
		{"GPT account alias", "desktop-alias", "GPT-6 Astra", "Doubao-Seed-2.0-pro", "GPT-6 Astra"},
		{"GPT global alias", "desktop-alias", "", "GPT-6 Astra", "GPT-6 Astra"},
		{"GPT Sol account alias", "desktop-alias", "GPT-5.6 Sol", "Doubao-Seed-2.0-pro", "GPT-5.6 Sol"},
		{"GPT Sol global alias", "desktop-alias", "", "GPT-5.6 Sol", "GPT-5.6 Sol"},
		{"Doubao account alias", "desktop-alias", "Doubao-Seed-2.0-pro", "GPT-6 Astra", "Doubao-Seed-2.0-pro"},
		{"Doubao global alias", "desktop-alias", "", "Doubao-Seed-2.0-pro", "Doubao-Seed-2.0-pro"},
		{"known GPT wins", "GPT-6 Astra", "Doubao-Seed-2.0-pro", "GLM-5.3", "GPT-6 Astra"},
		{"known Doubao wins", "Doubao-Seed-2.0-pro", "GPT-6 Astra", "GLM-5.3", "Doubao-Seed-2.0-pro"},
		{"MiniMax account alias", "desktop-alias", "MiniMax-M3", "GPT-6 Astra", "MiniMax-M3"},
		{"MiniMax global alias", "desktop-alias", "", "MiniMax-M3", "MiniMax-M3"},
		{"built-in default", "desktop-alias", "", "", joycode.DefaultModel},
		{"missing model", "", "GPT-6 Astra", "Doubao-Seed-2.0-pro", "GPT-6 Astra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var db *store.Store
			if tc.global != "" {
				var err error
				db, err = store.Open(filepath.Join(t.TempDir(), "model.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
				if err := db.SetSetting("default_model", tc.global); err != nil {
					t.Fatal(err)
				}
			}
			for _, stream := range []bool{false, true} {
				for _, controls := range []struct{ name, json, effort, thinking string }{
					{"reasoning object", `,"reasoning":{"effort":"max","summary":"auto"}`, "max", "enabled"},
					{"explicit effort wins", `,"reasoning_effort":"low","reasoning":{"effort":"max","summary":"auto"}`, "low", "enabled"},
					{"explicit disabled wins", `,"reasoning":{"effort":"max","summary":"auto"},"thinking":{"type":"disabled"}`, "max", "disabled"},
				} {
					t.Run(fmt.Sprintf("stream=%t/%s", stream, controls.name), func(t *testing.T) {
						calls := 0
						client := joycode.NewClient("synthetic", "synthetic")
						client.SetHTTPClient(&http.Client{Transport: resolvedModelRoundTripper(func(r *http.Request) (*http.Response, error) {
							calls++
							var body map[string]interface{}
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Fatal(err)
							}
							if body["model"] != tc.want {
								t.Errorf("model mismatch: outbound=%v resolved=%s", body["model"], tc.want)
							}
							function := r.URL.Query().Get("functionId")
							response := ""
							if joycode.IsResponsesAPIModel(tc.want) {
								if function != "responses_completions" || body["stream"] != true {
									t.Errorf("wrong Responses route/stream: %s %v", function, body)
								}
								reasoning, _ := body["reasoning"].(map[string]interface{})
								if reasoning["effort"] != controls.effort || reasoning["summary"] != "auto" {
									t.Errorf("reasoning fields lost: %v", body)
								}
								if body["max_output_tokens"] != float64(1024) {
									t.Errorf("explicit Responses limit lost: %v", body)
								}
								for _, key := range []string{"reasoning_effort", "thinking", "max_tokens"} {
									if _, exists := body[key]; exists {
										t.Errorf("Chat-only field %s leaked: %v", key, body)
									}
								}
								response = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
							} else {
								if function != "chat_completions" || body["stream"] != stream || body["reasoning_effort"] != controls.effort || body["max_tokens"] != float64(1024) {
									t.Errorf("wrong Chat route/fields: %s %v", function, body)
								}
								thinking, _ := body["thinking"].(map[string]interface{})
								if tc.want == "Doubao-Seed-2.0-pro" && thinking["type"] != controls.thinking {
									t.Errorf("Doubao thinking lost: %v", body)
								}
								if tc.want == "MiniMax-M3" && thinking["type"] != "adaptive" {
									t.Errorf("MiniMax adaptive changed: %v", body)
								}
								response = `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`
								if stream {
									response = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
								}
							}
							return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
						})})
						config := controls.json
						if tc.want == "MiniMax-M3" {
							config = fmt.Sprintf(`,"reasoning_effort":%q,"thinking":{"type":"adaptive"}`, controls.effort)
						}
						payload := fmt.Sprintf(`{"model":%q,"stream":%t,"max_tokens":1024,"messages":[{"role":"user","content":"test"}]%s}`, tc.model, stream, config)
						r := store.InitModel(store.InitAccountModel(httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(payload))))
						store.SetAccountDefaultModel(r, tc.account)
						mux := http.NewServeMux()
						NewServer(client, db).RegisterRoutes(mux)
						w := httptest.NewRecorder()
						mux.ServeHTTP(w, r)
						if calls != 1 || w.Code != 200 || !strings.Contains(w.Body.String(), `"content":"ok"`) || store.GetModel(r) != tc.want {
							t.Fatalf("calls=%d status=%d logged model=%s response=%s", calls, w.Code, store.GetModel(r), w.Body.String())
						}
					})
				}
			}
		})
	}
}
