package anthropic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/common"
	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
	"github.com/vibe-coding-labs/JoyCode2Api/pkg/store"
)

const chatEndpoint = "/api/saas/openai/v1/chat/completions"
const anthropicEndpoint = "/api/saas/anthropic/v1/messages"

// ClientResolver returns the appropriate joycode.Client for a request.
type ClientResolver func(r *http.Request) *joycode.Client

// Handler serves the Anthropic Messages API.
type Handler struct {
	Client   *joycode.Client
	Resolver ClientResolver
	store    *store.Store
}

// NewHandler creates a new Anthropic API handler.
func NewHandler(c *joycode.Client, s *store.Store) *Handler {
	return &Handler{Client: c, store: s}
}

func (h *Handler) getClient(r *http.Request) *joycode.Client {
	if h.Resolver != nil {
		return h.Resolver(r)
	}
	return h.Client
}

// RegisterRoutes registers the Anthropic Messages API endpoint.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/messages", h.handleMessages)
}

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(200)
		return
	}
	if r.Method != http.MethodPost {
		writeAnthropicError(w, 405, "method not allowed")
		return
	}

	var req MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reqLog(r).Error("decode anthropic request", "error", err)
		writeAnthropicError(w, 400, fmt.Sprintf("请求体解析失败: %s。请检查请求是否完整，或尝试开启新对话减少上下文长度。", err.Error()))
		return
	}
	defaultMaxTokens := 8192
	if h.store != nil {
		defaultMaxTokens = h.store.GetIntSetting("default_max_tokens", 8192)
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = defaultMaxTokens
	}
	if req.MaxTokens > 32768 {
		req.MaxTokens = 32768
	}

	accountDefault := store.GetAccountDefaultModel(r)
	systemDefault := ""
	if h.store != nil {
		systemDefault = h.store.GetSetting("default_model")
	}
	resolved := resolveModel(req.Model, accountDefault, systemDefault)
	store.SetModel(r, resolved)
	if resolved == "JoyCode-Base-V3" {
		writeAnthropicError(w, http.StatusBadRequest, "JoyCode-Base-V3 是代码补全模型，不支持聊天/工具会话。请选择目录中的聊天模型；不会自动替换为其他模型。")
		return
	}
	// Keep the existing max_tokens cap, but never let it make an enabled
	// native thinking budget invalid. Reject before any upstream call or SSE
	// headers; silently lowering the caller's budget would change its intent.
	if ClaudeNativeEnabled(h.store) && (IsNativeAnthropicModel(req.Model) || IsNativeAnthropicModel(resolved)) &&
		req.Thinking != nil && req.Thinking.Type == "enabled" && req.Thinking.BudgetTokens >= req.MaxTokens {
		writeAnthropicRequestError(w, fmt.Sprintf("thinking.budget_tokens (%d) 必须小于有效 max_tokens (%d，兼容上限 32768)。请显式降低 budget_tokens；请求未发送到上游。", req.Thinking.BudgetTokens, req.MaxTokens))
		return
	}
	reqLog(r).Info("anthropic request", "model", req.Model, "resolved", resolved, "stream", req.Stream, "max_tokens", req.MaxTokens, "messages", len(req.Messages), "tools", len(req.Tools))

	client := h.getClient(r)

	if req.Stream {
		h.handleStream(w, r, &req, client)
	} else {
		h.handleNonStream(w, r, &req, client)
	}
}

func (h *Handler) handleNonStream(w http.ResponseWriter, r *http.Request, req *MessageRequest, client *joycode.Client) {
	systemDefault := ""
	if h.store != nil {
		systemDefault = h.store.GetSetting("default_model")
	}
	if ClaudeNativeEnabled(h.store) && (IsNativeAnthropicModel(req.Model) || IsNativeAnthropicModel(resolveModel(req.Model, store.GetAccountDefaultModel(r), systemDefault))) {
		h.handleNativeAnthropicNonStream(w, r, req, client, systemDefault)
		return
	}
	// Preemptive truncation: estimate tokens and truncate before sending
	if rounds := PreemptiveTruncate(req); rounds < 0 {
		writeAnthropicRequestError(w, "上下文过长，自动截断后仍超出限制，请使用 /compact 或开启新对话。")
		return
	} else if rounds > 0 {
		slog.Warn("preemptive truncation applied (non-stream)", "rounds", rounds)
	}

	jcBody := TranslateRequest(req, store.GetAccountDefaultModel(r), systemDefault)
	logRequestDetails(r, "translated request (non-stream)", jcBody)
	if joycode.IsResponsesAPIModel(resolveModel(req.Model, store.GetAccountDefaultModel(r), systemDefault)) {
		// The upstream always returns SSE even for stream:false, so stream and
		// aggregate into a single Anthropic message.
		jcBody := TranslateRequest(req, store.GetAccountDefaultModel(r), systemDefault)
		jcBody["stream"] = true
		respBody := joycode.ChatToResponses(jcBody)
		resp, err := client.PostStream("/api/saas/openai/v1/responses", respBody)
		if err != nil {
			reqLog(r).Error("responses (non-stream) upstream error", "error", err)
			msg := err.Error()
			if isTimeoutError(err) {
				writeAnthropicError(w, 504, "上游服务响应超时，请稍后重试。原始错误: "+msg)
				return
			}
			writeAnthropicError(w, 500, msg)
			return
		}
		defer resp.Body.Close()

		result, err := readResponses(resp.Body, nil)
		if err != nil {
			writeAnthropicError(w, 500, err.Error())
			return
		}
		content := []ContentBlock{}
		if result.Text != "" {
			content = append(content, ContentBlock{Type: "text", Text: result.Text})
		}
		for _, tool := range result.Tools {
			content = append(content, tool.block())
		}
		if len(content) == 0 {
			content = []ContentBlock{{Type: "text", Text: ""}}
		}
		store.SetTokenUsage(r, result.Usage.InputTokens, result.Usage.OutputTokens)
		writeAnthropicJSON(w, 200, &MessageResponse{
			ID: NewMessageID(), Type: "message", Role: "assistant",
			Content: content, Model: req.Model, StopReason: &result.StopReason,
			Usage: result.Usage,
		})
		return
	}
	maxRetries := 3
	if h.store != nil {
		maxRetries = h.store.GetIntSetting("max_retries", 3)
	}
	var jcResp map[string]interface{}
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		jcResp, lastErr = client.Post(chatEndpoint, jcBody)
		if lastErr != nil {
			if isContextLimitError(lastErr.Error()) {
				// Progressive truncation on context limit
				if truncateMessages(req) {
					jcBody = TranslateRequest(req, store.GetAccountDefaultModel(r), systemDefault)
					reqLog(r).Warn("retrying with truncated messages (non-stream)", "attempt", attempt)
					continue
				}
				reqLog(r).Warn("context limit exceeded, cannot truncate further")
				writeAnthropicRequestError(w, "上下文长度超出模型限制，且无法进一步截断。请压缩对话历史或开启新对话。原始错误: "+lastErr.Error())
				return
			}
			reqLog(r).Error("non-stream retry error", "attempt", attempt, "max", maxRetries, "error", lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}
		break
	}

	if lastErr != nil {
		errMsg := lastErr.Error()
		if isContextLimitError(errMsg) {
			writeAnthropicRequestError(w, "上下文长度超出模型限制。请压缩对话历史或开启新对话。原始错误: "+errMsg)
			return
		}
		if isTimeoutError(lastErr) {
			reqLog(r).Error("upstream timeout after retries", "error", lastErr)
			writeAnthropicError(w, 504, "上游服务响应超时，请稍后重试。如果问题持续，请尝试减少上下文长度或开启新对话。原始错误: "+errMsg)
			return
		}
		if strings.Contains(errMsg, "content_filter") || strings.Contains(errMsg, "SENSITIVE_CONTENT") {
			reqLog(r).Warn("upstream content_filter (non-stream), returning detailed error")
			writeContentFilterError(w, errMsg)
			return
		}
		writeAnthropicError(w, 500, errMsg)
		return
	}
	// Check for content_filter in non-stream response
	if choices, ok := jcResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if fr, ok := choice["finish_reason"].(string); ok && fr == "content_filter" {
				reqLog(r).Warn("content_filter in non-stream response")
				raw, _ := json.Marshal(choice)
				writeContentFilterError(w, string(raw))
				return
			}
		}
	}
	resp, validationErr := validatedChatResponse(jcResp, req.Model)
	if validationErr != nil {
		writeAnthropicError(w, 500, validationErr.Error())
		return
	}
	if usage, ok := jcResp["usage"].(map[string]interface{}); ok {
		inTk, _ := usage["prompt_tokens"].(float64)
		outTk, _ := usage["completion_tokens"].(float64)
		store.SetTokenUsage(r, int(inTk), int(outTk))
	}
	writeAnthropicJSON(w, 200, resp)
}

// prependReader replays a buffered first line before reading from the underlying source.
type prependReader struct {
	first  []byte
	offset int
	source io.Reader
	body   io.ReadCloser
}

func (r *prependReader) Read(p []byte) (int, error) {
	if r.offset < len(r.first) {
		n := copy(p, r.first[r.offset:])
		r.offset += n
		return n, nil
	}
	return r.source.Read(p)
}

func (r *prependReader) Close() error {
	return r.body.Close()
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, req *MessageRequest, client *joycode.Client) {
	systemDefault := ""
	if h.store != nil {
		systemDefault = h.store.GetSetting("default_model")
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAnthropicError(w, 500, "streaming not supported")
		return
	}
	if ClaudeNativeEnabled(h.store) && (IsNativeAnthropicModel(req.Model) || IsNativeAnthropicModel(resolveModel(req.Model, store.GetAccountDefaultModel(r), systemDefault))) {
		h.handleNativeAnthropicStream(w, r, req, client, flusher, systemDefault)
		return
	}
	if joycode.IsResponsesAPIModel(resolveModel(req.Model, store.GetAccountDefaultModel(r), systemDefault)) {
		h.handleResponsesStream(w, r, req, client, flusher, systemDefault)
		return
	}

	jcBody := TranslateRequest(req, store.GetAccountDefaultModel(r), systemDefault)
	jcBody["stream"] = true
	logRequestDetails(r, "translated request (stream)", jcBody)

	// Preemptive truncation: estimate tokens and truncate before sending
	if rounds := PreemptiveTruncate(req); rounds < 0 {
		writeAnthropicRequestError(w, "上下文过长，自动截断后仍超出限制，请使用 /compact 或开启新对话。")
		return
	} else if rounds > 0 {
		reqLog(r).Warn("preemptive truncation applied (stream)", "rounds", rounds)
	}

	jcBody = TranslateRequest(req, store.GetAccountDefaultModel(r), systemDefault)
	jcBody["stream"] = true

	// Commit SSE headers + message_start early so we can send heartbeat ping
	// events while waiting for the upstream to respond. The JoyCode upstream
	// buffers the entire response (TTFB 10–30s for reasoning models); without
	// keepalive, Claude Code and other clients may time out during this gap.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

	msgID := NewMessageID()
	model := req.Model
	totalOutput := 0

	FormatSSE(w, "message_start", sseMessageStart{
		Type: "message_start",
		Message: MessageResponse{
			ID: msgID, Type: "message", Role: "assistant",
			Model: model, Content: []ContentBlock{}, Usage: Usage{},
		},
	})
	FormatSSE(w, "ping", ssePing{Type: "ping"})
	flusher.Flush()

	// Heartbeat: send periodic ping events while upstream is silent.
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
				FormatSSE(w, "ping", ssePing{Type: "ping"})
				flusher.Flush()
			}
		}
	}()

	// Connect with retry, progressive auto-truncate on context limit
	resp, err := h.connectStreamWithRetry(r, jcBody, client)
	for truncRound := 0; err != nil && isContextLimitError(err.Error()) && truncRound < maxTruncationRounds; truncRound++ {
		reqLog(r).Warn("stream context limit, truncating", "round", truncRound+1)
		if !truncateMessages(req) {
			break
		}
		jcBody = TranslateRequest(req, store.GetAccountDefaultModel(r), systemDefault)
		jcBody["stream"] = true
		resp, err = h.connectStreamWithRetry(r, jcBody, client)
	}
	close(stopHeartbeat)
	<-heartbeatDone
	if err != nil {
		errMsg := err.Error()
		if isContextLimitError(errMsg) {
			reqLog(r).Warn("context limit exceeded (stream), cannot proceed even after progressive truncation")
			writeStreamError(w, flusher, "上下文长度超出模型限制，已尝试自动截断但仍无法满足。请压缩对话历史或开启新对话。原始错误: "+errMsg)
			return
		}
		if isTimeoutError(err) {
			reqLog(r).Error("upstream timeout (stream) after retries", "error", err)
			writeStreamError(w, flusher, "上游服务响应超时，请稍后重试。如果问题持续，请尝试减少上下文长度或开启新对话。原始错误: "+errMsg)
			return
		}
		if strings.Contains(errMsg, "content_filter") || strings.Contains(errMsg, "SENSITIVE_CONTENT") {
			reqLog(r).Warn("upstream content_filter (stream), returning detailed error")
			writeStreamError(w, flusher, errMsg)
			return
		}
		reqLog(r).Error("stream failed after retries", "error", errMsg)
		writeStreamError(w, flusher, errMsg)
		return
	}
	defer resp.Body.Close()

	toolCalls := make(map[int]*completedTool)
	currentBlockIndex := 0
	textBlockStarted := false
	closeText := func() {
		if textBlockStarted {
			FormatSSE(w, "content_block_stop", sseContentBlockStop{Type: "content_block_stop", Index: currentBlockIndex})
			currentBlockIndex++
			textBlockStarted = false
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var streamInTk, streamOutTk int
	finishReason := ""
	var streamErr error

	for scanner.Scan() {
		line := unwrapNativeAnthropicSSE(scanner.Text())
		if line == "[DONE]" {
			break
		}
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:") {
			continue
		}
		if isUpstreamError(line) {
			streamErr = fmt.Errorf("upstream Chat stream error: %s", line)
			break
		}
		chunk := ParseStreamChunk(line)
		if chunk == nil {
			streamErr = fmt.Errorf("invalid upstream Chat stream event: %s", line)
			break
		}
		if chunk.Usage != nil {
			streamInTk = chunk.Usage.PromptTokens
			streamOutTk = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		// A finish_reason is terminal for the choice. Only usage/[DONE]
		// may follow it; don't emit tools until those trailers are consumed.
		if finishReason != "" {
			if choice.Delta.Content != "" || len(choice.Delta.ToolCalls) > 0 || (choice.FinishReason != nil && *choice.FinishReason != finishReason) {
				streamErr = fmt.Errorf("upstream Chat emitted content or conflicting finish_reason after completion")
				break
			}
			continue
		}
		for _, tc := range choice.Delta.ToolCalls {
			if tc.Index < 0 {
				streamErr = fmt.Errorf("invalid upstream tool index %d", tc.Index)
				break
			}
			tool := toolCalls[tc.Index]
			if tool == nil {
				tool = &completedTool{}
				toolCalls[tc.Index] = tool
			}
			if tc.ID != "" {
				if tool.ID != "" && tool.ID != tc.ID {
					streamErr = fmt.Errorf("conflicting upstream tool ID at index %d", tc.Index)
					break
				}
				tool.ID = tc.ID
			}
			if tc.Function.Name != "" {
				if tool.Name != "" && tool.Name != tc.Function.Name {
					streamErr = fmt.Errorf("conflicting upstream tool name at index %d", tc.Index)
					break
				}
				tool.Name = tc.Function.Name
			}
			tool.Arguments += tc.Function.Arguments
		}
		if streamErr != nil {
			break
		}
		if text := choice.Delta.Content; text != "" {
			if !textBlockStarted {
				textBlockStarted = true
				FormatSSE(w, "content_block_start", sseContentBlockStart{
					Type: "content_block_start", Index: currentBlockIndex,
					ContentBlock: ContentBlock{Type: "text", Text: ""},
				})
			}
			totalOutput += len(text)
			FormatSSE(w, "content_block_delta", sseContentBlockDelta{
				Type: "content_block_delta", Index: currentBlockIndex,
				Delta: deltaText{Type: "text_delta", Text: text},
			})
			flusher.Flush()
		}
		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
			if finishReason == "content_filter" {
				streamErr = fmt.Errorf("upstream Chat content_filter: %s", line)
				break
			}
		}
	}
	closeText()
	if streamInTk > 0 || streamOutTk > 0 {
		store.SetTokenUsage(r, streamInTk, streamOutTk)
	}
	if streamErr == nil && scanner.Err() != nil {
		streamErr = fmt.Errorf("reading upstream Chat stream: %w", scanner.Err())
	}
	if streamErr == nil && finishReason == "" {
		streamErr = fmt.Errorf("upstream Chat stream closed (EOF/[DONE]) before finish_reason")
	}
	stopReason := ""
	if streamErr == nil {
		stopReason, streamErr = chatStopReason(finishReason, len(toolCalls))
	}
	var tools []completedTool
	if streamErr == nil && stopReason != "max_tokens" {
		tools, streamErr = sortedChatTools(toolCalls)
	}
	if streamErr != nil {
		writeStreamError(w, flusher, streamErr.Error())
		return
	}
	for _, tool := range tools {
		emitCompletedTool(w, currentBlockIndex, tool)
		currentBlockIndex++
	}
	if currentBlockIndex == 0 {
		FormatSSE(w, "content_block_start", sseContentBlockStart{Type: "content_block_start", Index: 0, ContentBlock: ContentBlock{Type: "text", Text: ""}})
		FormatSSE(w, "content_block_stop", sseContentBlockStop{Type: "content_block_stop", Index: 0})
	}
	outputTokens := streamOutTk
	if outputTokens == 0 {
		outputTokens = totalOutput / 4
	}
	FormatSSE(w, "message_delta", sseMessageDelta{
		Type: "message_delta", Delta: deltaStop{StopReason: stopReason},
		Usage: struct {
			OutputTokens int `json:"output_tokens"`
		}{OutputTokens: outputTokens},
	})
	FormatSSE(w, "message_stop", sseMessageStop{Type: "message_stop"})
	flusher.Flush()

}

func (h *Handler) handleResponsesStream(w http.ResponseWriter, r *http.Request, req *MessageRequest, client *joycode.Client, flusher http.Flusher, systemDefault string) {
	jcBody := TranslateRequest(req, store.GetAccountDefaultModel(r), systemDefault)
	jcBody["stream"] = true
	body := joycode.ChatToResponses(jcBody)
	logRequestDetails(r, "translated responses request (stream)", body)

	// Commit SSE headers + message_start early so we can send heartbeat ping
	// events while waiting for the upstream to respond.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

	msgID := NewMessageID()
	model := req.Model

	FormatSSE(w, "message_start", sseMessageStart{
		Type: "message_start",
		Message: MessageResponse{
			ID: msgID, Type: "message", Role: "assistant",
			Model: model, Content: []ContentBlock{}, Usage: Usage{},
		},
	})
	FormatSSE(w, "ping", ssePing{Type: "ping"})
	flusher.Flush()

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
				FormatSSE(w, "ping", ssePing{Type: "ping"})
				flusher.Flush()
			}
		}
	}()

	resp, err := client.PostStream("/api/saas/openai/v1/responses", body)
	close(stopHeartbeat)
	<-heartbeatDone
	if err != nil {
		reqLog(r).Error("responses stream failed", "error", err)
		writeStreamError(w, flusher, err.Error())
		return
	}
	defer resp.Body.Close()

	currentBlockIndex := 0
	textBlockStarted := false
	result, readErr := readResponses(resp.Body, func(text string) {
		if !textBlockStarted {
			textBlockStarted = true
			FormatSSE(w, "content_block_start", sseContentBlockStart{
				Type: "content_block_start", Index: currentBlockIndex,
				ContentBlock: ContentBlock{Type: "text", Text: ""},
			})
		}
		FormatSSE(w, "content_block_delta", sseContentBlockDelta{
			Type: "content_block_delta", Index: currentBlockIndex,
			Delta: deltaText{Type: "text_delta", Text: text},
		})
		flusher.Flush()
	})
	if textBlockStarted {
		FormatSSE(w, "content_block_stop", sseContentBlockStop{Type: "content_block_stop", Index: currentBlockIndex})
		currentBlockIndex++
	}
	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 {
		store.SetTokenUsage(r, result.Usage.InputTokens, result.Usage.OutputTokens)
	}
	if readErr != nil {
		writeStreamError(w, flusher, readErr.Error())
		return
	}
	for _, tool := range result.Tools {
		emitCompletedTool(w, currentBlockIndex, tool)
		currentBlockIndex++
	}
	if currentBlockIndex == 0 && len(result.Tools) == 0 {
		// Preserve the guard: never append an empty block after real text.
		FormatSSE(w, "content_block_start", sseContentBlockStart{
			Type: "content_block_start", Index: currentBlockIndex,
			ContentBlock: ContentBlock{Type: "text", Text: ""},
		})
		FormatSSE(w, "content_block_stop", sseContentBlockStop{Type: "content_block_stop", Index: currentBlockIndex})
	}
	FormatSSE(w, "message_delta", sseMessageDelta{
		Type: "message_delta", Delta: deltaStop{StopReason: result.StopReason},
		Usage: struct {
			OutputTokens int `json:"output_tokens"`
		}{OutputTokens: result.Usage.OutputTokens},
	})
	FormatSSE(w, "message_stop", sseMessageStop{Type: "message_stop"})
	flusher.Flush()
}

func (h *Handler) handleNativeAnthropicStream(w http.ResponseWriter, r *http.Request, req *MessageRequest, client *joycode.Client, flusher http.Flusher, systemDefault string) {
	body := TranslateAnthropicRequest(req, store.GetAccountDefaultModel(r), systemDefault)
	logRequestDetails(r, "translated native anthropic request (stream)", body)

	// Commit SSE headers early so we can send heartbeat comment lines while
	// waiting for the upstream to respond (TTFB can be 10–30s for reasoning
	// models). SSE comment lines (": ...") are ignored by all compliant clients.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
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

	resp, err := h.connectNativeAnthropicStreamWithRetry(r, body, client)
	close(stopHeartbeat)
	<-heartbeatDone
	if err != nil {
		reqLog(r).Error("native anthropic stream failed after retries", "error", err)
		writeStreamError(w, flusher, err.Error())
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var inTk, outTk int
	pendingEvent := ""
	sawTerminal := false
	for scanner.Scan() {
		payload := unwrapNativeAnthropicSSE(scanner.Text())
		if payload == "" {
			continue
		}
		if strings.HasPrefix(payload, "event: ") {
			pendingEvent = strings.TrimSpace(strings.TrimPrefix(payload, "event: "))
			continue
		}
		if strings.HasPrefix(payload, "data: ") {
			payload = strings.TrimSpace(strings.TrimPrefix(payload, "data: "))
		}
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			fmt.Fprintln(w, "data: [DONE]")
			fmt.Fprintln(w)
			flusher.Flush()
			sawTerminal = true
			continue
		}
		if !strings.HasPrefix(payload, "{") {
			continue
		}
		eventName := pendingEvent
		if eventName == "" {
			eventName = nativeAnthropicEventType(payload)
		}
		if eventName == "message_stop" || eventName == "error" {
			sawTerminal = true
		}
		if eventName != "" {
			fmt.Fprintf(w, "event: %s\n", eventName)
		}
		fmt.Fprintf(w, "data: %s\n\n", payload)
		updateNativeAnthropicUsage(payload, &inTk, &outTk)
		pendingEvent = ""
		flusher.Flush()
	}
	// Surface a premature upstream close (no message_stop/[DONE]) as an error
	// rather than letting the client hang or treat it as done (issue #2).
	if !sawTerminal {
		if err := scanner.Err(); err != nil {
			reqLog(r).Error("native anthropic stream ended abnormally before message_stop", "error", err)
			writeStreamError(w, flusher, "读取上游流式响应失败，本次回复不完整，请重试。原始错误: "+err.Error())
		} else {
			reqLog(r).Warn("native anthropic stream closed before message_stop (premature upstream close)")
			writeStreamError(w, flusher, "上游在返回结束标记前断开了流式响应，本次回复可能不完整，请重试。")
		}
	} else if err := scanner.Err(); err != nil {
		reqLog(r).Error("native anthropic stream scanner error", "error", err)
	}
	if inTk > 0 || outTk > 0 {
		store.SetTokenUsage(r, inTk, outTk)
	}
}

func (h *Handler) handleNativeAnthropicNonStream(w http.ResponseWriter, r *http.Request, req *MessageRequest, client *joycode.Client, systemDefault string) {
	body := TranslateAnthropicRequest(req, store.GetAccountDefaultModel(r), systemDefault)
	logRequestDetails(r, "translated native anthropic request (non-stream)", body)

	resp, err := h.connectNativeAnthropicStreamWithRetry(r, body, client)
	if err != nil {
		reqLog(r).Error("native anthropic non-stream failed after retries", "error", err)
		writeAnthropicError(w, 500, err.Error())
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	content := []ContentBlock{}
	var current *ContentBlock
	stopReason := "end_turn"
	var inTk, outTk int

	for scanner.Scan() {
		payload := unwrapNativeAnthropicSSE(scanner.Text())
		if payload == "" || strings.HasPrefix(payload, "event: ") {
			continue
		}
		if strings.HasPrefix(payload, "data: ") {
			payload = strings.TrimSpace(strings.TrimPrefix(payload, "data: "))
		}
		if payload == "" || payload == "[DONE]" || !strings.HasPrefix(payload, "{") {
			continue
		}
		if isUpstreamError(payload) {
			writeAnthropicError(w, 500, payload)
			return
		}
		updateNativeAnthropicUsage(payload, &inTk, &outTk)

		var event struct {
			Type         string          `json:"type"`
			ContentBlock ContentBlock    `json:"content_block"`
			Delta        json.RawMessage `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "content_block_start":
			block := event.ContentBlock
			current = &block
		case "content_block_delta":
			if current == nil {
				block := ContentBlock{Type: "text"}
				current = &block
			}
			var delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			}
			if err := json.Unmarshal(event.Delta, &delta); err != nil {
				continue
			}
			switch delta.Type {
			// Bedrock-style native endpoints emit the serialized delta as
			// {"type":"text","text":...} rather than {"type":"text_delta",...};
			// treat both the same (issue #5).
			case "text_delta", "text":
				current.Type = "text"
				current.Text += delta.Text
			case "input_json_delta":
				if current.Input == nil {
					current.Input = map[string]interface{}{}
				}
				if delta.PartialJSON != "" {
					var input interface{}
					if err := json.Unmarshal([]byte(delta.PartialJSON), &input); err == nil {
						current.Input = input
					}
				}
			}
		case "content_block_stop":
			if current != nil {
				content = append(content, *current)
				current = nil
			}
		case "message_delta":
			var delta struct {
				StopReason string `json:"stop_reason"`
			}
			var wrapper struct {
				Delta deltaStop `json:"delta"`
			}
			if err := json.Unmarshal([]byte(payload), &wrapper); err == nil && wrapper.Delta.StopReason != "" {
				stopReason = wrapper.Delta.StopReason
			} else if err := json.Unmarshal(event.Delta, &delta); err == nil && delta.StopReason != "" {
				stopReason = delta.StopReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		reqLog(r).Error("native anthropic non-stream scanner error", "error", err)
		writeAnthropicError(w, 500, err.Error())
		return
	}
	if current != nil {
		content = append(content, *current)
	}
	if len(content) == 0 {
		content = []ContentBlock{{Type: "text", Text: ""}}
	}
	if inTk > 0 || outTk > 0 {
		store.SetTokenUsage(r, inTk, outTk)
	}
	writeAnthropicJSON(w, 200, &MessageResponse{
		ID:         NewMessageID(),
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      req.Model,
		StopReason: &stopReason,
		Usage: Usage{
			InputTokens:  inTk,
			OutputTokens: outTk,
		},
	})
}

func (h *Handler) connectNativeAnthropicStreamWithRetry(r *http.Request, body map[string]interface{}, client *joycode.Client) (*http.Response, error) {
	maxRetries := 3
	if h.store != nil {
		maxRetries = h.store.GetIntSetting("max_retries", 3)
	}
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.PostAnthropicStream(anthropicEndpoint, body)
		if err != nil {
			lastErr = err
			reqLog(r).Error("native anthropic stream connect error", "attempt", attempt, "max", maxRetries, "error", err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}
		br := bufio.NewReaderSize(resp.Body, 64*1024)
		firstLine, err := br.ReadString('\n')
		if err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("read first line: %w", err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}
		if err := nativeAnthropicLineError(firstLine); err != nil {
			resp.Body.Close()
			lastErr = err
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}
		resp.Body = &prependReader{first: []byte(firstLine), source: br, body: resp.Body}
		return resp, nil
	}
	return nil, lastErr
}

func unwrapNativeAnthropicSSE(line string) string {
	trimmed := strings.TrimSpace(line)
	for {
		next := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if next == trimmed {
			return trimmed
		}
		trimmed = next
	}
}

func nativeAnthropicEventType(payload string) string {
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}
	return event.Type
}

func nativeAnthropicLineError(line string) error {
	payload := unwrapNativeAnthropicSSE(line)
	if payload == "" || payload == "[DONE]" || !strings.HasPrefix(payload, "{") {
		return nil
	}
	if isUpstreamError(payload) {
		return fmt.Errorf("%s", payload)
	}
	return nil
}

func updateNativeAnthropicUsage(payload string, inputTokens, outputTokens *int) {
	if payload == "" || payload == "[DONE]" || !strings.HasPrefix(payload, "{") {
		return
	}
	var event struct {
		Type    string `json:"type"`
		Message struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return
	}
	if event.Message.Usage.InputTokens > 0 {
		*inputTokens = event.Message.Usage.InputTokens
	}
	if event.Message.Usage.OutputTokens > 0 {
		*outputTokens = event.Message.Usage.OutputTokens
	}
	if event.Usage.InputTokens > 0 {
		*inputTokens = event.Usage.InputTokens
	}
	if event.Usage.OutputTokens > 0 {
		*outputTokens = event.Usage.OutputTokens
	}
}

// connectStreamWithRetry attempts to connect to upstream with retries.
// Peeks at the first SSE line to detect errors before returning the response.
func (h *Handler) connectStreamWithRetry(r *http.Request, jcBody map[string]interface{}, client *joycode.Client) (*http.Response, error) {
	maxRetries := 3
	if h.store != nil {
		maxRetries = h.store.GetIntSetting("max_retries", 3)
	}
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := client.PostStream(chatEndpoint, jcBody)
		if err != nil {
			lastErr = err
			reqLog(r).Error("stream connect error", "attempt", attempt, "max", maxRetries, "error", err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}

		br := bufio.NewReaderSize(resp.Body, 64*1024)
		firstLine, readErr := br.ReadString('\n')
		if readErr != nil && firstLine == "" {
			resp.Body.Close()
			lastErr = fmt.Errorf("read first line: %w", readErr)
			reqLog(r).Error("stream read first line", "attempt", attempt, "max", maxRetries, "error", lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}

		dataContent := unwrapNativeAnthropicSSE(firstLine)
		if isUpstreamError(dataContent) {
			resp.Body.Close()
			lastErr = fmt.Errorf("upstream error: %s", dataContent)
			logUpstreamError(r, attempt, maxRetries, dataContent)
			if isContextLimitError(dataContent) {
				return nil, lastErr
			}
			// SENSITIVE_CONTENT errors are deterministic — retrying is pointless
			if strings.Contains(dataContent, "SENSITIVE_CONTENT") {
				return nil, lastErr
			}
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
			continue
		}

		// Check first line for content_filter (content + finish_reason in same chunk)
		if filtered, chunkData := extractContentFilterInfo(dataContent); filtered {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s", chunkData)
			reqLog(r).Warn("content_filter detected in first chunk, not retrying", "chunk", truncate(chunkData, 300))
			return nil, lastErr
		}

		// Leave subsequent lines to the stream parser. Peeking another line
		// used to discard its bytes and read error when it lacked a newline.
		originalBody := resp.Body
		var source io.Reader = br
		if readErr != nil {
			// ReadString already consumed the error; replay it after its bytes.
			source = completionReadError{err: readErr}
		}
		resp.Body = &prependReader{
			first:  []byte(firstLine),
			source: source,
			body:   originalBody,
		}
		reqLog(r).Info("stream connected", "attempt", attempt)
		return resp, nil
	}
	return nil, fmt.Errorf("stream failed after %d attempts: %w", maxRetries, lastErr)
}

// isTimeoutError checks if the error is caused by an upstream timeout.
func isTimeoutError(err error) bool {
	return common.IsTimeoutError(err)
}

// isContextLimitError checks if the upstream error indicates context length exceeded.
func isContextLimitError(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "context length") ||
		strings.Contains(lower, "context window") ||
		strings.Contains(lower, "token limit") ||
		strings.Contains(lower, "tokens exceeded") ||
		strings.Contains(lower, "input length") ||
		strings.Contains(lower, "model_context_window_exceeded") ||
		strings.Contains(lower, "prompt length") ||
		strings.Contains(lower, "max_input_tokens")
}

// extractContentFilterInfo checks if a SSE data line contains content_filter finish_reason.
// Returns whether content_filter was detected and the raw data line for error reporting.
func extractContentFilterInfo(line string) (bool, string) {
	if line == "" || line == "[DONE]" {
		return false, ""
	}
	var parsed struct {
		Choices []struct {
			FinishReason *string         `json:"finish_reason"`
			Message      json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return false, ""
	}
	for _, c := range parsed.Choices {
		if c.FinishReason != nil && *c.FinishReason == "content_filter" {
			return true, line
		}
	}
	return false, ""
}

func isUpstreamError(line string) bool {
	if line == "" || line == "[DONE]" {
		return false
	}
	var parsed struct {
		Choices []interface{} `json:"choices"`
		Error   interface{}   `json:"error"`
		Code    interface{}   `json:"code"`
		Status  string        `json:"status"`
		Msg     string        `json:"msg"`
	}
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return false
	}
	if len(parsed.Choices) > 0 {
		return false
	}
	return parsed.Error != nil || parsed.Code != nil || parsed.Status != "" || parsed.Msg != ""
}

func truncate(s string, maxLen int) string {
	return common.Truncate(s, maxLen)
}

func writeAnthropicJSON(w http.ResponseWriter, code int, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("writeAnthropicJSON: marshal failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	w.Write(b)
}

// writeStreamError emits an Anthropic `error` SSE event mid-stream. Used when an
// already-started stream is truncated, so the client surfaces a failure (and can
// retry) instead of treating a partial response as a clean completion (issue #2).
func writeStreamError(w http.ResponseWriter, flusher http.Flusher, msg string) {
	FormatSSE(w, "error", map[string]interface{}{
		"type":  "error",
		"error": map[string]string{"type": "api_error", "message": msg},
	})
	flusher.Flush()
}

func writeAnthropicError(w http.ResponseWriter, code int, msg string) {
	writeAnthropicJSON(w, code, map[string]interface{}{
		"type":  "error",
		"error": map[string]string{"type": "api_error", "message": msg},
	})
}

// writeContentFilterError forwards the upstream content_filter response verbatim.
// Uses invalid_request_error type so Claude Code treats it as non-retryable.
func writeContentFilterError(w http.ResponseWriter, msgs ...string) {
	msg := "content_filter"
	if len(msgs) > 0 && msgs[0] != "" {
		msg = msgs[0]
	}
	writeAnthropicJSON(w, 400, map[string]interface{}{
		"type":  "error",
		"error": map[string]string{"type": "invalid_request_error", "message": msg},
	})
}

func writeAnthropicRequestError(w http.ResponseWriter, msg string) {
	writeAnthropicJSON(w, 400, map[string]interface{}{
		"type":  "error",
		"error": map[string]string{"type": "invalid_request_error", "message": msg},
	})
}
