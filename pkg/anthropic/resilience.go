package anthropic

import (
	"encoding/json"
	"strings"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/common"
	"github.com/vibe-coding-labs/JoyCode2Api/pkg/joycode"
)

// Upstream fault tolerance: when the single upstream rejects a request over
// content we control (stale thinking signatures, unsupported images), repair
// the request and retry once instead of letting the conversation die. When
// the fault is over parameters (e.g. thinking budget constraints), the error
// is passed through verbatim — silently rewriting the caller's intent is not
// our call (see handler.go's budget rejection for the same philosophy).
//
// All rewriting happens on raw JSON blocks so unknown fields (citations,
// provider-specific extras) survive byte-for-byte for every kept block.

const unsupportedImagePlaceholder = "[Unsupported Image]"

// isThinkingSignatureError reports whether an upstream error is caused by an
// incompatible or invalid thinking signature block (cc-switch's "Thinking 签名
// 整流" equivalent: signature chains are provider-bound, so a rejected
// signature is only ever a replayed history block, never new intent).
func isThinkingSignatureError(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "signature") &&
		(strings.Contains(lower, "thinking") || strings.Contains(lower, "reasoning"))
}

// isImageUnsupportedError reports whether an upstream error means the model
// does not accept image input (declared text-only capability or a hard
// upstream rejection).
func isImageUnsupportedError(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "image") &&
		(strings.Contains(lower, "not support") ||
			strings.Contains(lower, "unsupported") ||
			strings.Contains(lower, "does not") ||
			strings.Contains(lower, "unable to") ||
			strings.Contains(lower, "invalid image") ||
			strings.Contains(lower, "no vision"))
}

// isUpstreamPromptError reports whether an upstream error means the translated
// request contained an empty / unusable prompt — GLM-family gateways' code 1213
// "未正常接收到prompt参数". Observed trigger: truncation leaves a user message
// whose blocks are all orphaned tool_results, translate.go converts it to
// {"role":"user","content":""} and the upstream rejects the whole request.
func isUpstreamPromptError(body string) bool {
	return strings.Contains(body, "1213") ||
		strings.Contains(body, "未正常接收到prompt参数")
}

// isToolsLimitError reports whether the upstream rejected the request because
// the number of tool declarations exceeds its cap — GLM-family gateways reply
// "tools 数量超过上限 128" (invalid_request_error, param=tools). The request
// itself is fine; only the tool schema count is over. Observed with Claude
// Code sessions mounting 160+ tool schemas through the proxy (2026-11-15,
// request_logs 147889-147980: 60 consecutive 500s, session dead in the water).
// The one-shot repair drops tool schemas and retries, same as the HTML-shell
// oversized-payload path.
func isToolsLimitError(body string) bool {
	return strings.Contains(body, "tools 数量超过上限") ||
		(strings.Contains(strings.ToLower(body), `"param":"tools"`) &&
			strings.Contains(strings.ToLower(body), "invalid_request_error"))
}

// isHtmlShellError reports whether the upstream answered with its default HTML
// landing page instead of JSON. The JoyCode gateway (JD's jfe-lb layer) does
// this when it rejects a request outright — observed with very large payloads
// (hundreds of K tokens of context + tool schemas). Retrying identically only
// burns the caller's time; the one-shot repair drops tool schemas, which
// dominate the payload in the observed failures, and retries once.
func isHtmlShellError(body string) bool {
	return common.IsHtmlShellResponse(stripParseErrorPrefix(body))
}

// stripParseErrorPrefix removes the "upstream html shell response (parse
// error: ...)" prefix the client attaches, so the classifier sees the raw
// body. Falls back to the input when the prefix is absent.
func stripParseErrorPrefix(body string) string {
	const marker = "): "
	for _, prefix := range []string{"upstream html shell response (parse error", "invalid JSON response (parse error"} {
		if strings.HasPrefix(body, prefix) {
			if i := strings.Index(body, marker); i >= 0 && i < len(prefix)+120 {
				rest := body[i+len(marker):]
				return rest
			}
		}
	}
	return body
}

// dropToolSchemas removes the request's tool declarations (keeping tool_use /
// tool_result history blocks intact) so a payload rejected by the gateway for
// size can be retried. Tool-calling is degraded for this one request, which
// is strictly better than a hard failure. Returns true when something was
// dropped.
func dropToolSchemas(req *MessageRequest) bool {
	if len(req.Tools) == 0 {
		return false
	}
	req.Tools = nil
	return true
}

// stripEmptyUserMessages removes user messages that translate to an empty
// payload (no text, no image, only orphaned tool_result blocks) so a request
// that hits code 1213 can be retried without them. Returns the number of
// messages removed; 0 means nothing changed.
func stripEmptyUserMessages(req *MessageRequest) int {
	removed := 0
	toolUseIDs := map[string]bool{}
	for _, m := range req.Messages {
		if m.Role != "assistant" {
			continue
		}
		blocks, ok := decodeRawBlocks(m.Content)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if blockType(b) == "tool_use" {
				var tb struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(b, &tb) == nil && tb.ID != "" {
					toolUseIDs[tb.ID] = true
				}
			}
		}
	}
	kept := req.Messages[:0:0]
	for _, m := range req.Messages {
		if m.Role == "user" && !userMessageHasPayload(m, toolUseIDs) {
			removed++
			continue
		}
		kept = append(kept, m)
	}
	if removed > 0 {
		req.Messages = kept
	}
	return removed
}

// userMessageHasPayload reports whether a user message carries content that
// survives translation: text, an image, or a tool_result whose author is
// still in the conversation.
func userMessageHasPayload(m MessageParam, toolUseIDs map[string]bool) bool {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return strings.TrimSpace(s) != ""
	}
	blocks, ok := decodeRawBlocks(m.Content)
	if !ok {
		return len(m.Content) > 0
	}
	for _, b := range blocks {
		switch blockType(b) {
		case "text":
			var tb struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(b, &tb) == nil && strings.TrimSpace(tb.Text) != "" {
				return true
			}
		case "image":
			return true
		case "tool_result":
			var trb struct {
				ToolUseID string `json:"tool_use_id"`
			}
			if json.Unmarshal(b, &trb) == nil && trb.ToolUseID != "" && toolUseIDs[trb.ToolUseID] {
				return true
			}
		}
	}
	return false
}

// stripThinkingBlocks removes thinking (and redacted_thinking) blocks from
// every assistant message, keeping all other blocks verbatim. Returns the
// number of blocks removed; 0 means nothing changed.
func stripThinkingBlocks(req *MessageRequest) int {
	removed := 0
	for i := range req.Messages {
		if req.Messages[i].Role != "assistant" {
			continue
		}
		blocks, ok := decodeRawBlocks(req.Messages[i].Content)
		if !ok {
			continue
		}
		kept := make([]json.RawMessage, 0, len(blocks))
		changed := false
		for _, b := range blocks {
			if t := blockType(b); t == "thinking" || t == "redacted_thinking" {
				removed++
				changed = true
				continue
			}
			kept = append(kept, b)
		}
		if changed {
			req.Messages[i].Content = marshalBlocks(kept)
		}
	}
	return removed
}

// replaceUnsupportedImages replaces every image block — including inside
// tool_result content — with a text placeholder so a vision-incapable model
// can still continue the conversation. Returns the number of blocks replaced.
func replaceUnsupportedImages(req *MessageRequest) int {
	replaced := 0
	for i := range req.Messages {
		blocks, ok := decodeRawBlocks(req.Messages[i].Content)
		if !ok {
			continue
		}
		changed := false
		for j, b := range blocks {
			switch blockType(b) {
			case "image":
				blocks[j] = placeholderBlock()
				replaced++
				changed = true
			case "tool_result":
				if n, newBlock := replaceImagesInToolResult(b); n > 0 {
					replaced += n
					blocks[j] = newBlock
					changed = true
				}
			}
		}
		if changed {
			req.Messages[i].Content = marshalBlocks(blocks)
		}
	}
	return replaced
}

// replaceImagesInToolResult rewrites image blocks nested in a tool_result's
// content payload. Returns the replacement count and the rewritten block;
// when nothing changed, the original block is returned untouched.
func replaceImagesInToolResult(block json.RawMessage) (int, json.RawMessage) {
	// Decode into a map so non-content tool_result fields survive the rewrite.
	var m map[string]json.RawMessage
	if json.Unmarshal(block, &m) != nil {
		return 0, block
	}
	inner, ok := m["content"]
	if !ok {
		return 0, block
	}
	nested, ok := decodeRawBlocks(inner)
	if !ok {
		return 0, block
	}
	replaced := 0
	for j, nb := range nested {
		if blockType(nb) == "image" {
			nested[j] = placeholderBlock()
			replaced++
		}
	}
	if replaced == 0 {
		return 0, block
	}
	rewritten, err := json.Marshal(nested)
	if err != nil {
		return 0, block
	}
	m["content"] = json.RawMessage(rewritten)
	out, err := json.Marshal(m)
	if err != nil {
		return 0, block
	}
	return replaced, out
}

// StripImagesForTextOnlyModel preemptively replaces image blocks when the
// resolved model is in the confirmed text-only registry (cc-switch's
// "纯文本模型预判" equivalent). Returns the number of blocks replaced; 0 means
// no image was present or the model is vision-capable / unknown.
func StripImagesForTextOnlyModel(req *MessageRequest, resolvedModel string) int {
	if !joycode.IsKnownTextOnlyModel(resolvedModel) {
		return 0
	}
	return replaceUnsupportedImages(req)
}

// decodeRawBlocks parses a content payload as an array of raw JSON blocks.
// Returns false for plain-string or malformed content (nothing to walk).
func decodeRawBlocks(raw json.RawMessage) ([]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, false
	}
	return blocks, true
}

// blockType peeks at a raw JSON content block's "type" field.
func blockType(raw json.RawMessage) string {
	var t struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &t) != nil {
		return ""
	}
	return t.Type
}

func placeholderBlock() json.RawMessage {
	return json.RawMessage(`{"type":"text","text":"` + unsupportedImagePlaceholder + `"}`)
}

func marshalBlocks(blocks []json.RawMessage) json.RawMessage {
	out, err := json.Marshal(blocks)
	if err != nil {
		// Cannot happen for already-valid raw JSON; keep old content if it does.
		return nil
	}
	return out
}

// unsupportedEffortFallback reports whether the upstream rejected the
// reasoning effort "none" — the value the proxy derives from thinking:disabled
// — and pins the request to the lowest effort that model does accept.
//
// The effort the model accepts is read from the upstream's own rejection
// message, so a model that gains or loses "none" needs no code change here.
// 'none' is the one effort whose intent ("do not reason") a model may be
// unable to express at all; the upstream names the remaining levels, and the
// lowest of them is the closest fulfillment. Every other rejected effort is
// the caller's own parameter and passes through untouched.
func unsupportedEffortFallback(errMsg string, req *MessageRequest) (string, bool) {
	rejected, lowest, ok := joycode.UnsupportedEffort(errMsg)
	if !ok || rejected != "none" {
		return "", false
	}
	if req.OutputConfig == nil {
		req.OutputConfig = &OutputConfig{}
	}
	req.OutputConfig.Effort = lowest
	return lowest, true
}
