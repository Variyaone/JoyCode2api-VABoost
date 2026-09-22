package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
)

const (
	// Upstream model context window size (JoyCode's ModelArts limit).
	// JoyAI/OpenAI-family paths are capped at 192K; native Claude paths are
	// negotiated upstream (200K or 1M), so this proxy floor applies only to
	// non-native models.
	contextWindowSize = 196608
	// Safety margin: start truncation when estimated tokens exceed this ratio of contextWindowSize
	preemptiveThresholdRatio = 0.85
	// Rough approximation: 1 token ≈ 3.5 bytes for mixed Chinese/English code content
	bytesPerToken = 3.5
	// Maximum truncation rounds before giving up
	maxTruncationRounds = 5
	// Image patch constants. Vision models charge per image via a base + N
	// patches. Using OpenAI/Anthropic conventions: 85 base + 170/patch.
	// Cap each image so a screenshot-heavy conversation stays truncatable.
	imageBaseTokens   = 85
	imagePatchToken   = 170
	imagePatchPixels  = 512
	maxTokensPerImage = imageBaseTokens + imagePatchToken*25 // ~4K screenshot
)

// estimateTokens gives a rough token count estimate for the request messages.
// Uses byte-length / bytesPerToken as approximation for text content.
// Image blocks are estimated by patch count (OpenAI/Anthropic convention)
// instead of by raw base64 byte length, so a screenshot-heavy turn estimates
// in the thousands of tokens rather than hundreds of thousands.
func estimateTokens(req *MessageRequest) int {
	totalBytes := int64(0)
	imageTokens := 0
	if req.System != nil {
		totalBytes += int64(len(req.System))
	}
	for _, m := range req.Messages {
		contentBytes, imgTokens := estimateMessageTokens(m.Content)
		totalBytes += contentBytes
		imageTokens += imgTokens
	}
	if totalBytes == 0 && imageTokens == 0 {
		return 0
	}
	return int(float64(totalBytes)/bytesPerToken) + imageTokens
}

// estimateMessageTokens splits a message's content into text-byte cost and
// vision-token cost. Content may be a plain string, a JSON array of blocks,
// or (for provider errors) arbitrary JSON.
func estimateMessageTokens(raw json.RawMessage) (textBytes int64, imageTokens int) {
	// Fast path: plain string content.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return int64(len(s)), 0
	}

	// Array of content blocks.
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		slog.Debug("estimateMessageTokens: unmarshal blocks failed, using raw byte length",
			"err", err,
			"raw_len", len(raw),
		)
		// Unknown shape — fall back to raw length (old behaviour).
		return int64(len(raw)), 0
	}

	for _, b := range blocks {
		tb, it := estimateBlockTokens(b)
		textBytes += tb
		imageTokens += it
	}
	return textBytes, imageTokens
}

// estimateBlockTokens handles a single content block, recursing into nested
// tool_result content (which can hold images, strings, or further block arrays).
func estimateBlockTokens(b contentBlock) (textBytes int64, imageTokens int) {
	switch b.Type {
	case "image":
		return 0, estimateImageTokens(b.Source)
	case "tool_result":
		return estimateToolResultTokens(b.Content)
	default:
		// text, tool_use, thinking, etc. — count the serialised size.
		if jb, err := json.Marshal(b); err == nil {
			return int64(len(jb)), 0
		}
	}
	return 0, 0
}

// estimateToolResultTokens walks a tool_result's content payload. Anthropic
// allows string, or array of {type:text|image|...} blocks; tool output often
// carries image blocks here (Read/screenshot). Unknown shapes fall back to
// raw-length estimation so we never silently zero out.
func estimateToolResultTokens(content json.RawMessage) (textBytes int64, imageTokens int) {
	if len(content) == 0 {
		return 0, 0
	}
	// Identical shape handling as a full message: string, block array, or raw.
	return estimateMessageTokens(content)
}

// estimateImageTokens converts an Anthropic image block source into an
// approximate vision-token cost. Base64 sources carry no intrinsic pixel
// dimensions, so we infer a scale from the decoded payload size relative to
// a typical screenshot (~300KB base64 ≈ 1080p) and cap the result. URL data
// sources decode in-place; plain-URL sources fall back to the max patch cap
// because we cannot inspect the remote payload synchronously.
func estimateImageTokens(source json.RawMessage) int {
	if len(source) == 0 {
		return maxTokensPerImage
	}
	var src struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(source, &src); err != nil {
		return maxTokensPerImage
	}

	var payloadBase64 string
	switch src.Type {
	case "base64":
		payloadBase64 = src.Data
	case "url":
		// data:image/...;base64,XXXX — strip the prefix and treat like base64.
		const marker = ";base64,"
		if idx := strings.Index(src.URL, marker); idx >= 0 {
			payloadBase64 = src.URL[idx+len(marker):]
		} else {
			return maxTokensPerImage
		}
	default:
		return maxTokensPerImage
	}

	if len(payloadBase64) == 0 {
		return maxTokensPerImage
	}

	// Approximate decoded image payload size (base64 inflates by 4/3).
	decodedBytes := int64(len(payloadBase64)) * 3 / 4
	if decodedBytes == 0 {
		return maxTokensPerImage
	}

	// Decode actual pixel dimensions from the image header — this avoids the
	// encoding-format sensitivity of byte-size calibration (raw BMP payloads
	// would otherwise overcount 4-6x vs the same PNG image).
	if w, h := pixelDimsFromBase64(payloadBase64); w > 0 && h > 0 {
		// Cap pixels to prevent integer overflow on enormous inputs
		const maxDim = 100000
		if w > maxDim {
			w = maxDim
		}
		if h > maxDim {
			h = maxDim
		}
		xTiles := (w + imagePatchPixels - 1) / imagePatchPixels
		yTiles := (h + imagePatchPixels - 1) / imagePatchPixels
		patches := xTiles * yTiles
		tokens := imageBaseTokens + imagePatchToken*patches
		if tokens > maxTokensPerImage {
			tokens = maxTokensPerImage
		}
		return tokens
	}

	// Fallback: unknown format → assume heavy; cap.
	return maxTokensPerImage
}

// pixelDimsFromBase64 extracts width/height from a base64-encoded image
// without a full decode. We decode up to 64KB of base64 (≈48KB decoded) to
// cover JPEG APP1 EXIF segments that phone-camera photos commonly pad to
// 30-64KB before SOF0; PNG/BMP header parsing stays within the first bytes.
// Returns (0, 0) for unrecognised formats; callers fall back to a cap.
func pixelDimsFromBase64(b64 string) (int, int) {
	const maxScan = 65536
	if len(b64) > maxScan {
		b64 = b64[:maxScan]
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		// Padding stripped mid-chunk: pad and retry the longest 4-aligned prefix.
		trim := len(b64) - (len(b64) % 4)
		if trim < 24 {
			return 0, 0
		}
		raw, err = base64.StdEncoding.DecodeString(b64[:trim])
		if err != nil {
			return 0, 0
		}
	}
	if len(raw) < 4 {
		return 0, 0
	}

	// PNG: 89 50 4E 47 0D 0A 1A 0A | IHDR(13 bytes from offset 8): len(4) "IHDR" W(4) H(4)
	if len(raw) >= 24 && raw[0] == 0x89 && raw[1] == 0x50 && raw[2] == 0x4E && raw[3] == 0x47 {
		if string(raw[12:16]) != "IHDR" {
			return 0, 0
		}
		w := int(raw[16])<<24 | int(raw[17])<<16 | int(raw[18])<<8 | int(raw[19])
		h := int(raw[20])<<24 | int(raw[21])<<16 | int(raw[22])<<8 | int(raw[23])
		return w, h
	}

	// BMP: 42 4D ("BM") + width(4 signed LE @18) + height(4 signed LE @22).
	// Negative height = top-down bitmap.
	if len(raw) >= 26 && raw[0] == 0x42 && raw[1] == 0x4D {
		w := int32(raw[18]) | int32(raw[19])<<8 | int32(raw[20])<<16 | int32(raw[21])<<24
		h := int32(raw[22]) | int32(raw[23])<<8 | int32(raw[24])<<16 | int32(raw[25])<<24
		if h < 0 {
			h = -h
		}
		if w < 0 {
			w = -w
		}
		return int(w), int(h)
	}

	// JPEG: FF D8, then scan for SOF0/SOF1/SOF2 (FF C0/C1/C2) whose body starts
	// with precision(1) H(2) W(2). Need i+8 within bounds to read W.
	if len(raw) >= 9 && raw[0] == 0xFF && raw[1] == 0xD8 {
		for i := 2; i+8 < len(raw); {
			if raw[i] != 0xFF {
				i++
				continue
			}
			marker := raw[i+1]
			// ITU T.81 permits any number of 0xFF fill bytes before a marker,
			// and stuffed FF 00 sequences appear inside scan data; skip them.
			if marker == 0xFF || marker == 0x00 {
				i++
				continue
			}
			if marker == 0xC0 || marker == 0xC1 || marker == 0xC2 {
				h := int(raw[i+5])<<8 | int(raw[i+6])
				w := int(raw[i+7])<<8 | int(raw[i+8])
				return w, h
			}
			if i+4 > len(raw) {
				break
			}
			segLen := int(raw[i+2])<<8 | int(raw[i+3])
			if segLen < 2 {
				break
			}
			i += 2 + segLen
		}
		return 0, 0
	}
	return 0, 0
}

// PreemptiveTruncate checks if the request likely exceeds the model's context limit
// and proactively truncates before sending. Returns the number of truncation rounds performed.
// Returns -1 if truncation failed to bring estimated tokens below threshold.
func PreemptiveTruncate(req *MessageRequest) int {
	window := float64(contextWindowSize)
	ratio := float64(preemptiveThresholdRatio)
	threshold := int(window * ratio)

	rounds := 0
	for rounds < maxTruncationRounds {
		estimated := estimateTokens(req)
		if estimated <= threshold {
			if rounds > 0 {
				slog.Info("preemptive truncation complete", "rounds", rounds, "estimated_tokens", estimated, "threshold", threshold)
			}
			return rounds
		}
		if !truncateMessages(req) {
			slog.Warn("preemptive truncation: cannot truncate further", "estimated_tokens", estimated, "threshold", threshold, "rounds", rounds)
			return -1
		}
		rounds++
		slog.Warn("preemptive truncation round", "round", rounds, "estimated_tokens_before", estimated, "threshold", threshold)
	}
	// Check if we actually got below threshold
	if estimateTokens(req) > threshold {
		slog.Warn("preemptive truncation exhausted rounds without reaching threshold")
		return -1
	}
	return rounds
}

// findToolPairBoundary adjusts cutEnd backward if it would split a tool_use/tool_result pair.
func findToolPairBoundary(messages []MessageParam, cutEnd int) int {
	if cutEnd <= 1 || cutEnd >= len(messages) {
		return cutEnd
	}
	// Tool calls and results may be separated by mid-turn system messages or
	// parallel tool results. Pair by ID rather than assuming role alternation.
	uses := make(map[string]int)
	for i, message := range messages {
		if message.Role != "assistant" {
			continue
		}
		var blocks []contentBlock
		if json.Unmarshal(message.Content, &blocks) != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type == "tool_use" && block.ID != "" {
				uses[block.ID] = i
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for i := cutEnd; i < len(messages); i++ {
			var blocks []contentBlock
			if json.Unmarshal(messages[i].Content, &blocks) != nil {
				continue
			}
			for _, block := range blocks {
				if block.Type != "tool_result" {
					continue
				}
				if use, ok := uses[block.ToolUseID]; ok && use >= 1 && use < cutEnd {
					cutEnd = use
					changed = true
				}
			}
		}
	}
	if cutEnd <= 1 {
		return cutEnd
	}
	prevRole := messages[cutEnd-1].Role
	curRole := messages[cutEnd].Role

	// If we're splitting between assistant(tool_use) and user(tool_result), include both
	if prevRole == "assistant" && curRole == "user" {
		var blocks []contentBlock
		if json.Unmarshal(messages[cutEnd-1].Content, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "tool_use" {
					if cutEnd-1 >= 1 {
						return cutEnd - 1
					}
					if cutEnd+2 < len(messages) {
						return cutEnd + 2
					}
				}
			}
		}
	}

	// Skip orphaned tool_result whose tool_use was removed
	if curRole == "user" {
		var blocks []contentBlock
		if json.Unmarshal(messages[cutEnd].Content, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "tool_result" {
					if cutEnd+1 < len(messages) {
						return cutEnd + 1
					}
				}
			}
		}
	}

	return cutEnd
}

// truncateMessages removes the oldest messages from the middle of the
// conversation, keeping the first message + a truncation notice + the last
// portion of messages. Each call removes ~40% of remaining messages.
// Returns true if truncation was performed.
func truncateMessages(req *MessageRequest) bool {
	n := len(req.Messages)
	if n <= 4 {
		return false
	}

	keepFirst := 1
	// Remove 40% of messages each round (more aggressive than before)
	keepLast := int(float64(n) * 0.6)
	if keepLast < 4 {
		keepLast = 4
	}
	cutEnd := n - keepLast
	if cutEnd <= keepFirst {
		// Even more aggressive: only keep first + last 2
		keepLast = 2
		cutEnd = n - keepLast
		if cutEnd <= keepFirst {
			return false
		}
	}

	// Ensure cutEnd lands on an even index (user) for valid conversation sequence
	if cutEnd%2 != 0 {
		cutEnd++
	}
	if cutEnd >= n {
		return false
	}

	// Adjust cutEnd to avoid splitting tool_use/tool_result pairs
	cutEnd = findToolPairBoundary(req.Messages, cutEnd)
	if cutEnd <= keepFirst || cutEnd >= n {
		return false
	}

	removed := cutEnd - keepFirst
	notice := "[System: Earlier conversation messages have been auto-truncated to fit within the model's context window. Some earlier context is now missing. Continue with the remaining conversation.]"
	noticeBytes, _ := json.Marshal(notice)

	var truncated []MessageParam
	truncated = append(truncated, req.Messages[:keepFirst]...)
	truncated = append(truncated, MessageParam{
		Role:    "assistant",
		Content: json.RawMessage(noticeBytes),
	})
	truncated = append(truncated, req.Messages[cutEnd:]...)

	slog.Warn("auto-truncated messages for context limit",
		"original_count", n,
		"truncated_count", len(truncated),
		"removed", removed,
		"kept_first", keepFirst,
		"kept_last", n-cutEnd,
	)

	req.Messages = truncated
	return true
}
