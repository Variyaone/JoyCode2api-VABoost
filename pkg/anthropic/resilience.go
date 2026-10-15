package anthropic

import (
	"encoding/json"
	"strings"

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
