package openai

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/common"
	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
	"github.com/vibe-coding-labs/JoyCode2Api/pkg/store"
)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("decode chat request", "error", err)
		writeError(w, 400, fmt.Sprintf("请求体解析失败: %s。请检查请求是否完整，或尝试开启新对话减少上下文长度。", err.Error()))
		return
	}
	systemDefault := ""
	if s.store != nil {
		systemDefault = s.store.GetSetting("default_model")
	}
	model := ResolveModel(req.Model, store.GetAccountDefaultModel(r), systemDefault)
	req.Model = model
	store.SetModel(r, model)
	if model == "JoyCode-Base-V3" {
		writeError(w, http.StatusBadRequest, "JoyCode-Base-V3 是代码补全模型，不支持聊天/工具会话。请选择目录中的聊天模型；不会自动替换为其他模型。")
		return
	}
	client := s.getClient(r)
	if joycode.IsResponsesAPIModel(model) {
		s.handleResponsesModel(w, r, client, &req, model)
		return
	}
	jcBody := TranslateRequest(&req)
	if req.Stream {
		s.handleStreamChat(w, r, client, jcBody, model)
	} else {
		s.handleNonStreamChat(w, r, client, jcBody, model)
	}
}

// handleResponsesModel routes GPT-family models through the OpenAI Responses
// API (functionId=responses_completions). The upstream chat/completions path
// rejects these models with error 1032.
func (s *Server) handleResponsesModel(w http.ResponseWriter, r *http.Request, client *joycode.Client, req *ChatRequest, model string) {
	body := joycode.ChatToResponses(TranslateRequest(req))
	if req.Stream {
		s.handleResponsesStream(w, r, client, body, model)
	} else {
		s.handleResponsesNonStream(w, r, client, body, model)
	}
}

func (s *Server) handleResponsesNonStream(w http.ResponseWriter, r *http.Request, client *joycode.Client, body map[string]interface{}, model string) {
	// The upstream ignores stream:false and always returns SSE, so we always
	// stream and aggregate the result into a single completion object.
	body["stream"] = true
	resp, err := client.PostStream("/api/saas/openai/v1/responses", body)
	if err != nil {
		slog.Error("responses non-stream upstream error", "model", model, "error", err)
		msg := err.Error()
		code := 500
		if isTimeoutError(msg) {
			code = 504
			msg = "上游服务响应超时，请稍后重试。原始错误: " + msg
		}
		writeError(w, code, msg)
		return
	}
	defer resp.Body.Close()

	st := &ResponsesStreamState{Model: model, toolCalls: map[string]*responsesToolCall{}}
	var text strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		for strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		if isResponsesError(line) {
			writeError(w, 500, line)
			return
		}
		for _, chunkJSON := range st.Feed(line) {
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(chunkJSON), &chunk) == nil && len(chunk.Choices) > 0 {
				if chunk.Choices[0].Delta.Content != "" {
					text.WriteString(chunk.Choices[0].Delta.Content)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Error("responses non-stream read error", "model", model, "error", err)
		writeError(w, 500, err.Error())
		return
	}
	// Collect tool calls from the state
	var toolCalls []interface{}
	for _, tc := range st.sortedToolCalls() {
		toolCalls = append(toolCalls, map[string]interface{}{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]interface{}{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		})
	}
	message := map[string]interface{}{"role": "assistant", "content": text.String()}
	finish := "stop"
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		finish = "tool_calls"
	}
	inTk, outTk := st.Usage()
	if inTk > 0 || outTk > 0 {
		store.SetTokenUsage(r, inTk, outTk)
	}
	writeJSON(w, 200, map[string]interface{}{
		"id":      "chatcmpl-" + newShortID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []interface{}{map[string]interface{}{
			"index":         0,
			"message":       message,
			"finish_reason": finish,
		}},
		"usage": map[string]interface{}{
			"prompt_tokens":     inTk,
			"completion_tokens": outTk,
		},
	})
}

func (s *Server) handleResponsesStream(w http.ResponseWriter, r *http.Request, client *joycode.Client, body map[string]interface{}, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Error("streaming not supported by response writer")
		return
	}
	body["stream"] = true

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "close")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer close(heartbeatDone)
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}()

	resp, err := client.PostStream("/api/saas/openai/v1/responses", body)
	if err != nil {
		close(stopHeartbeat)
		<-heartbeatDone
		slog.Error("responses stream upstream error", "model", model, "error", err)
		msg := err.Error()
		if isTimeoutError(msg) {
			msg = "上游服务响应超时，请稍后重试。原始错误: " + msg
		}
		fmt.Fprintf(w, "data: {\"error\":{\"message\":\"%s\"}}\n\n", msg)
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}
	defer resp.Body.Close()
	close(stopHeartbeat)
	<-heartbeatDone

	st := &ResponsesStreamState{Model: model, toolCalls: map[string]*responsesToolCall{}}
	sawDone := false
	sawError := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// The upstream double-wraps SSE: "data: event: x" / "data: data: {...}".
		for strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		if isResponsesError(line) {
			sawError = true
			slog.Warn("responses stream upstream error", "model", model, "payload", common.Truncate(line, 300))
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
			continue
		}
		for _, chunk := range st.Feed(line) {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Error("responses stream read error", "model", model, "error", err)
	}
	if !sawError {
		for _, chunk := range st.Finalize() {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
	}
	if !sawDone {
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
	if inTk, outTk := st.Usage(); inTk > 0 || outTk > 0 {
		store.SetTokenUsage(r, inTk, outTk)
	}
}

// isResponsesError detects upstream error payloads in a Responses SSE line.
func isResponsesError(line string) bool {
	var parsed struct {
		Type   string      `json:"type"`
		Error  interface{} `json:"error"`
		Code   interface{} `json:"code"`
		Status string      `json:"status"`
	}
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return false
	}
	if parsed.Type == "error" || parsed.Error != nil || parsed.Status == "FAILED_RESPONSE" {
		return true
	}
	if parsed.Code != nil && parsed.Type == "" {
		return true
	}
	return false
}

func (s *Server) handleNonStreamChat(w http.ResponseWriter, r *http.Request, client *joycode.Client, jcBody map[string]interface{}, model string) {
	resp, err := client.Post("/api/saas/openai/v1/chat/completions", jcBody)
	if err != nil {
		slog.Error("chat non-stream upstream error", "model", model, "error", err)
		msg := err.Error()
		code := 500
		if isTimeoutError(msg) {
			code = 504
			msg = "上游服务响应超时，请稍后重试。原始错误: " + msg
		}
		writeError(w, code, msg)
		return
	}
	if usage, ok := resp["usage"].(map[string]interface{}); ok {
		inTk, _ := usage["prompt_tokens"].(float64)
		outTk, _ := usage["completion_tokens"].(float64)
		store.SetTokenUsage(r, int(inTk), int(outTk))
	}
	writeJSON(w, 200, TranslateResponse(resp, model))
}

func (s *Server) handleStreamChat(w http.ResponseWriter, r *http.Request, client *joycode.Client, jcBody map[string]interface{}, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Error("streaming not supported by response writer")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "close")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

	// Start a heartbeat goroutine. The upstream JoyCode API buffers the entire
	// response before sending anything (TTFB can be 10–30s for reasoning models).
	// Without keepalive, downstream clients (Claude Code, OpenAI clients) may
	// time out or show "no response" during this gap. SSE comment lines (": ...")
	// are part of the spec and ignored by all compliant clients.
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer close(heartbeatDone)
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}()

	streamStart := time.Now()
	resp, err := client.PostStream("/api/saas/openai/v1/chat/completions", jcBody)
	if err != nil {
		close(stopHeartbeat)
		<-heartbeatDone
		slog.Error("chat stream upstream error", "model", model, "error", err)
		msg := err.Error()
		if isTimeoutError(msg) {
			msg = "上游服务响应超时，请稍后重试。原始错误: " + msg
		}
		fmt.Fprintf(w, "data: {\"error\":{\"message\":\"%s\"}}\n\n", msg)
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}
	defer resp.Body.Close()
	close(stopHeartbeat)
	<-heartbeatDone
	slog.Info("stream: connected to upstream", "model", model, "ttfb_ms", time.Since(streamStart).Milliseconds())

	// Pipe JoyCode SSE response line-by-line — already OpenAI-compatible format.
	// Using bufio.Scanner (not raw Read) ensures each SSE event is forwarded
	// as soon as it arrives, without buffering multiple events into one write.
	// Also extract usage tokens from the final chunk for dashboard stats.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var inTk, outTk int
	sawDone := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Empty line separates SSE events; forward as-is.
			w.Write([]byte("\n"))
			flusher.Flush()
			continue
		}
		if strings.Contains(line, "[DONE]") {
			sawDone = true
		}
		w.Write([]byte(line))
		w.Write([]byte("\n"))
		flusher.Flush()
		// Extract usage from data lines (the final chunk carries usage stats)
		if strings.HasPrefix(line, "data: ") && !strings.Contains(line, "[DONE]") {
			var chunk struct {
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk) == nil && chunk.Usage != nil {
				inTk = chunk.Usage.PromptTokens
				outTk = chunk.Usage.CompletionTokens
			}
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Error("chat stream read error", "model", model, "error", err)
	}
	// Ensure the stream terminates with [DONE]. Some upstream responses omit it
	// (e.g. premature close, certain error paths), leaving clients like
	// CherryStudio hanging in "generating" state. Always send a final [DONE].
	if !sawDone {
		slog.Warn("stream ended without [DONE], sending terminator", "model", model)
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}
	if inTk > 0 || outTk > 0 {
		store.SetTokenUsage(r, inTk, outTk)
	}
}

func isTimeoutError(msg string) bool {
	return common.IsTimeoutError(errors.New(msg))
}
