package anthropic

import (
	"encoding/json"
	"testing"
)

func TestEstimateImageTokens_RealPNG(t *testing.T) {
	// Real-format PNG: 1920×1080 → tiles = ceil(1920/512)×ceil(1080/512) = 4×3 = 12 patches
	// Tokens = 85 + 170*12 = 2125, capped at 4255 → 2125.
	block := mkImageBlockReal(t, mkPNGHeader(t, 1920, 1080), "image/png")
	tb, tok := estimateMessageTokens(json.RawMessage("[" + string(block) + "]"))
	if tb != 0 {
		t.Errorf("expected 0 text bytes for image block, got %d", tb)
	}
	want := imageBaseTokens + imagePatchToken*12 // 85 + 170*12 = 2125
	if tok != want {
		t.Errorf("PNG 1920x1080 estimated to %d, want %d", tok, want)
	}
}

func TestEstimateImageTokens_RealBMPTopDown(t *testing.T) {
	// BMP with negative height (top-down): should parse as positive 1920×1080.
	block := mkImageBlockReal(t, mkBMP(t, 1920, -1080), "image/bmp")
	_, tok := estimateMessageTokens(json.RawMessage("[" + string(block) + "]"))
	want := imageBaseTokens + imagePatchToken*12
	if tok != want {
		t.Errorf("BMP top-down 1920x1080 estimated to %d, want %d", tok, want)
	}
}

func TestEstimateImageTokens_RealJPEG(t *testing.T) {
	// JPEG 512×512 → 1 patch → 85+170 = 255
	block := mkImageBlockReal(t, mkJPEG(t, 512, 512), "image/jpeg")

	// Direct call to pixelDimsFromBase64 to see if header parse works
	src := struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	}{}
	var blockMap map[string]interface{}
	if err := json.Unmarshal(block, &blockMap); err != nil {
		t.Fatal(err)
	}
	srcB, _ := json.Marshal(blockMap["source"])
	if err := json.Unmarshal(srcB, &src); err != nil {
		t.Fatal(err)
	}
	t.Logf("src.Data length=%d first80=%q", len(src.Data), src.Data[:min(80, len(src.Data))])
	w, h := pixelDimsFromBase64(src.Data)
	t.Logf("pixelDimsFromBase64 = (%d, %d)", w, h)

	_, tok := estimateMessageTokens(json.RawMessage("[" + string(block) + "]"))
	t.Logf("estimateMessageTokens = %d", tok)
	want := imageBaseTokens + imagePatchToken*1
	if tok != want {
		t.Errorf("JPEG 512x512 estimated to %d, want %d", tok, want)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestEstimateImageTokens_UnknownFormatCaps(t *testing.T) {
	// mkImageBlockBytes writes filler bytes with no format magic — should fall
	// back to the per-image cap (conservative overcount).
	block := mkImageBlockBytes(t, 300)
	_, tok := estimateMessageTokens(json.RawMessage("[" + string(block) + "]"))
	if tok != maxTokensPerImage {
		t.Errorf("unparseable image should cap at %d, got %d", maxTokensPerImage, tok)
	}
}

func TestEstimateImageTokens_HugeImageCapped(t *testing.T) {
	// 8K × 8K = 16×16 patches = 256; would exceed cap, clamp to 25.
	block := mkImageBlockReal(t, mkPNGHeader(t, 8192, 8192), "image/png")
	_, tok := estimateMessageTokens(json.RawMessage("[" + string(block) + "]"))
	if tok != maxTokensPerImage {
		t.Errorf("8K image should cap at %d, got %d", maxTokensPerImage, tok)
	}
}

func TestEstimateTokens_ImageDoesNotPanic(t *testing.T) {
	// Real PNG header → parses → 1080p image → 2125 tokens.
	img := mkImageBlockReal(t, mkPNGHeader(t, 1920, 1080), "image/png")
	msg := MessageParam{
		Role:    "user",
		Content: json.RawMessage(`[{"type":"text","text":"hi"},` + string(img) + `]`),
	}
	req := &MessageRequest{Messages: []MessageParam{msg}}
	got := estimateTokens(req)
	if got < 2000 || got > 2500 {
		t.Errorf("1080p image turn estimated at %d tokens, want ~2125", got)
	}
}

func TestEstimateTokens_TextOnlyUnchanged(t *testing.T) {
	msg := MessageParam{
		Role:    "user",
		Content: json.RawMessage(`[{"type":"text","text":"hello world"}]`),
	}
	before := estimateTokens(&MessageRequest{Messages: []MessageParam{msg}})

	// Pure text estimate should be ~len/3.5 — unchanged from pre-fix.
	jsonSize := int64(len(msg.Content))
	want := int(float64(jsonSize) / bytesPerToken)
	if before != want {
		t.Errorf("text-only estimate changed: got %d, want %d", before, want)
	}
}

func TestEstimateTokens_ToolResultImageNested(t *testing.T) {
	// 1080p PNG inside a tool_result → 12 patches ≈ 2125 tokens.
	imgBlock := mkImageBlockReal(t, mkPNGHeader(t, 1920, 1080), "image/png")
	tr := map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "tu_1",
		"content":     json.RawMessage("[" + string(imgBlock) + "]"),
	}
	trB, _ := json.Marshal(tr)
	// Anthropic wraps tool_result in the message's content array:
	// messages[i].content = [ { type:"tool_result", ... } ]
	msg := MessageParam{Role: "user", Content: json.RawMessage("[" + string(trB) + "]")}
	_, tok := estimateMessageTokens(msg.Content)
	if tok >= 1900 && tok <= 2400 {
		return
	}
	t.Errorf("tool_result image estimated to %d, want ~2125", tok)
}

func TestPreemptiveTruncate_ImageHeavyConversationFits(t *testing.T) {
	// Realistic Soumina handover: 14 tool_result images, each a 1080p PNG.
	// Old byte-based estimator: 196K estimated → truncate fails → proxy 400.
	// New patch-based estimator: ~14*2125 + text ≈ 30K → well under 192K window.
	msgs := []MessageParam{{Role: "user", Content: json.RawMessage(`"start"`)}, {Role: "assistant", Content: json.RawMessage(`"ok"`)}}
	for i := 0; i < 14; i++ {
		img := mkImageBlockReal(t, mkPNGHeader(t, 1920, 1080), "image/png")
		inner, _ := json.Marshal(map[string]interface{}{
			"type":        "tool_result",
			"tool_use_id": "tu_img",
			"content":     json.RawMessage("[" + string(img) + "]"),
		})
		outer := json.RawMessage("[" + string(inner) + "]")
		msgs = append(msgs, MessageParam{Role: "user", Content: outer})
		msgs = append(msgs, MessageParam{Role: "assistant", Content: json.RawMessage(`"analysis"`)})
	}
	req := &MessageRequest{Messages: msgs}
	rounds := PreemptiveTruncate(req)
	if rounds != 0 {
		t.Fatalf("image-heavy conversation should fit without truncation, got %d rounds", rounds)
	}
}
