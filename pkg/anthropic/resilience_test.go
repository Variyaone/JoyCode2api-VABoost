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

type resilienceRoundTripper func(*http.Request) (*http.Response, error)

func (f resilienceRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func sseResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

const resilienceChatStream = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"

const resilienceNativeStream = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_r\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

// signatureImagePayload has one assistant thinking block (with signature),
// one text block, and one user image — enough to exercise both repairs.
const signatureImagePayload = `{
	"model":"%s",
	"stream":%t,
	"max_tokens":8192,
	"messages":[
		{"role":"assistant","content":[
			{"type":"thinking","thinking":"prior","signature":"sig-from-other-provider"},
			{"type":"text","text":"prior reply"}
		]},
		{"role":"user","content":[
			{"type":"text","text":"look at this"},
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}
		]}
	]
}`

// TestThinkingSignatureStrippedAndRetried: first upstream call rejects with a
// signature error; the proxy strips thinking blocks and retries — the client
// gets a normal completion instead of a dead conversation.
func TestThinkingSignatureStrippedAndRetried(t *testing.T) {
	for _, tc := range []struct {
		name, model, rejectBody string
		native                  bool
	}{
		{"chat", "GLM-5.3", `{"error":{"message":"invalid signature in thinking block","code":1024}}`, false},
		{"native", "Claude-Opus-5", `{"type":"error","error":{"type":"invalid_request_error","message":"Decoding error: thinking signature verification failed"}}`, true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				calls := 0
				client := joycode.NewClient("synthetic", "synthetic")
				client.SetHTTPClient(&http.Client{Transport: resilienceRoundTripper(func(r *http.Request) (*http.Response, error) {
					calls++
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if calls == 1 {
						return nil, fmt.Errorf("API error 400: %s", tc.rejectBody)
					}
					// The retried body must carry no thinking blocks and no signature.
					encoded, _ := json.Marshal(body["messages"])
					if strings.Contains(string(encoded), "thinking") || strings.Contains(string(encoded), "signature") {
						t.Fatalf("thinking survived the repair: %s", encoded)
					}
					if tc.native {
						if r.URL.Query().Get("functionId") != "anthropic_completions" {
							t.Fatalf("wrong route: %s", r.URL)
						}
						return sseResponse(resilienceNativeStream), nil
					}
					if r.URL.Query().Get("functionId") != "chat_completions" {
						t.Fatalf("wrong route: %s", r.URL)
					}
					if stream {
						return sseResponse(resilienceChatStream), nil
					}
					return sseResponse(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
				})})
				mux := http.NewServeMux()
				NewHandler(client, nil).RegisterRoutes(mux)
				payload := fmt.Sprintf(signatureImagePayload, tc.model, stream)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
				if calls != 2 {
					t.Fatalf("expected 2 upstream calls (reject + repaired retry), got %d; response=%s", calls, w.Body.String())
				}
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"ok"`) {
					t.Fatalf("repaired retry did not complete: status=%d response=%s", w.Code, w.Body.String())
				}
			})
		}
	}
}

// TestImageDowngradeOnUpstreamRejection: upstream rejects image input; the
// proxy replaces image blocks (including tool_result nested) with
// [Unsupported Image] and retries once.
func TestImageDowngradeOnUpstreamRejection(t *testing.T) {
	payload := `{
		"model":"Kimi-K3",
		"stream":%t,
		"max_tokens":8192,
		"messages":[
			{"role":"user","content":[
				{"type":"text","text":"look"},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}},
				{"type":"tool_result","tool_use_id":"tu_1","content":[
					{"type":"text","text":"file"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"BB=="}}
				]}
			]}
		]
	}`
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			calls := 0
			client := joycode.NewClient("synthetic", "synthetic")
			client.SetHTTPClient(&http.Client{Transport: resilienceRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if calls == 1 {
					return nil, fmt.Errorf("API error 400: {\"error\":{\"message\":\"image input is not supported by this model\",\"code\":1025}}")
				}
				encoded, _ := json.Marshal(body["messages"])
				s := string(encoded)
				if strings.Contains(s, "image") {
					t.Fatalf("image survived the downgrade: %s", s)
				}
				if !strings.Contains(s, unsupportedImagePlaceholder) {
					t.Fatalf("placeholder missing: %s", s)
				}
				if stream {
					return sseResponse(resilienceChatStream), nil
				}
				return sseResponse(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
			})})
			mux := http.NewServeMux()
			NewHandler(client, nil).RegisterRoutes(mux)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(fmt.Sprintf(payload, stream))))
			if calls != 2 || w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"ok"`) {
				t.Fatalf("calls=%d status=%d response=%s", calls, w.Code, w.Body.String())
			}
		})
	}
}

// TestTextOnlyModelPreemptiveStrip: a confirmed text-only model gets images
// replaced before the first upstream call (preemptive, no error needed).
func TestTextOnlyModelPreemptiveStrip(t *testing.T) {
	for _, model := range []string{"JoyAI-Code-1.5", "GLM-5.3", "DeepSeek-V4-Pro"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				calls := 0
				client := joycode.NewClient("synthetic", "synthetic")
				client.SetHTTPClient(&http.Client{Transport: resilienceRoundTripper(func(r *http.Request) (*http.Response, error) {
					calls++
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					encoded, _ := json.Marshal(body["messages"])
					if strings.Contains(string(encoded), "image") {
						t.Fatalf("image reached text-only model %s: %s", model, encoded)
					}
					if calls != 1 {
						t.Fatalf("preemptive strip should need exactly 1 call, got %d", calls)
					}
					if stream {
						return sseResponse(resilienceChatStream), nil
					}
					return sseResponse(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
				})})
				mux := http.NewServeMux()
				NewHandler(client, nil).RegisterRoutes(mux)
				payload := fmt.Sprintf(signatureImagePayload, model, stream)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"text":"ok"`) {
					t.Fatalf("status=%d response=%s", w.Code, w.Body.String())
				}
			})
		}
	}
}

// TestVisionModelKeepsImages: vision-capable and unknown models keep their
// images when the upstream accepts them (no over-eager stripping).
func TestVisionModelKeepsImages(t *testing.T) {
	for _, model := range []string{"Kimi-K3", "unknown-alias-model"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				calls := 0
				client := joycode.NewClient("synthetic", "synthetic")
				client.SetHTTPClient(&http.Client{Transport: resilienceRoundTripper(func(r *http.Request) (*http.Response, error) {
					calls++
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					encoded, _ := json.Marshal(body["messages"])
					if strings.Contains(string(encoded), "image_url") == false && model == "Kimi-K3" {
						t.Fatalf("vision model lost its image: %s", encoded)
					}
					if calls != 1 {
						t.Fatalf("expected exactly 1 call, got %d", calls)
					}
					if stream {
						return sseResponse(resilienceChatStream), nil
					}
					return sseResponse(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
				})})
				mux := http.NewServeMux()
				NewHandler(client, nil).RegisterRoutes(mux)
				payload := fmt.Sprintf(signatureImagePayload, model, stream)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
				if w.Code != 200 {
					t.Fatalf("status=%d response=%s", w.Code, w.Body.String())
				}
			})
		}
	}
}

// TestBudgetErrorPassesThrough: parameter errors are never auto-repaired —
// the caller must fix its own budget (existing philosophy, now guarded by a
// regression test).
func TestBudgetErrorPassesThrough(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			calls := 0
			client := joycode.NewClient("synthetic", "synthetic")
			client.SetHTTPClient(&http.Client{Transport: resilienceRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				return nil, fmt.Errorf("API error 400: {\"error\":{\"message\":\"thinking.budget_tokens must be at least 1024 and less than max_tokens\",\"type\":\"invalid_request_error\"}}")
			})})
			mux := http.NewServeMux()
			NewHandler(client, nil).RegisterRoutes(mux)
			payload := fmt.Sprintf(`{"model":"Claude-Opus-5","stream":%t,"max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":512},"messages":[{"role":"user","content":"test"}]}`, stream)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
			if calls == 0 {
				t.Fatalf("budget error never reached upstream — was it silently repaired? response=%s", w.Body.String())
			}
			body := w.Body.String()
			if !strings.Contains(body, "budget_tokens") {
				t.Fatalf("budget constraint was not passed through verbatim: %s", body)
			}
		})
	}
}

// TestErrorClassifiers pins the classifier patterns against both real-world
// upstream phrasings and near-miss negatives.
func TestErrorClassifiers(t *testing.T) {
	sigPos := []string{
		"invalid thinking signature",
		"Decoding error: signature verification failed for reasoning block",
		"thinking block signature mismatch",
	}
	sigNeg := []string{
		"signature check passed",
		"invalid request error",
		"context length exceeded",
	}
	for _, s := range sigPos {
		if !isThinkingSignatureError(s) {
			t.Errorf("isThinkingSignatureError(%q) = false, want true", s)
		}
	}
	for _, s := range sigNeg {
		if isThinkingSignatureError(s) {
			t.Errorf("isThinkingSignatureError(%q) = true, want false", s)
		}
	}
	imgPos := []string{
		"image input is not supported",
		"this model does not support image",
		"unsupported image format",
		"you have no vision capability for this image",
	}
	imgNeg := []string{
		"image generated successfully",
		"the image looks fine",
		"context window exceeded",
	}
	for _, s := range imgPos {
		if !isImageUnsupportedError(s) {
			t.Errorf("isImageUnsupportedError(%q) = false, want true", s)
		}
	}
	for _, s := range imgNeg {
		if isImageUnsupportedError(s) {
			t.Errorf("isImageUnsupportedError(%q) = true, want false", s)
		}
	}
}

// TestStripThinkingBlocksPreservesOtherBlocks: kept blocks must survive
// byte-for-byte, including unknown extra fields.
func TestStripThinkingBlocksPreservesOtherBlocks(t *testing.T) {
	req := &MessageRequest{Messages: []MessageParam{{
		Role: "assistant",
		Content: json.RawMessage(`[
			{"type":"thinking","thinking":"secret","signature":"sig"},
			{"type":"text","text":"hi","citations":[{"unknown":"keep"}],"future_field":123},
			{"type":"tool_use","id":"tu_9","name":"check","input":{"n":1}}
		]`),
	}}}
	if n := stripThinkingBlocks(req); n != 1 {
		t.Fatalf("removed=%d, want 1", n)
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(req.Messages[0].Content, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("kept %d blocks, want 2: %s", len(blocks), req.Messages[0].Content)
	}
	if !strings.Contains(string(blocks[0]), `"citations":[{"unknown":"keep"}]`) || !strings.Contains(string(blocks[0]), `"future_field":123`) {
		t.Fatalf("unknown fields lost: %s", blocks[0])
	}
	if !strings.Contains(string(blocks[1]), `"tool_use"`) {
		t.Fatalf("tool_use lost: %s", blocks[1])
	}
}
