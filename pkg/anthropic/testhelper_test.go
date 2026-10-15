package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// mkPNGHeader builds a minimal PNG header (signature + IHDR) for the given
// dimensions — no IDAT or CRC. Enough for pixelDimsFromBase64 to parse.
func mkPNGHeader(t *testing.T, w, h int) []byte {
	t.Helper()
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R',
		byte(w >> 24 & 0xff), byte(w >> 16 & 0xff), byte(w >> 8 & 0xff), byte(w & 0xff),
		byte(h >> 24 & 0xff), byte(h >> 16 & 0xff), byte(h >> 8 & 0xff), byte(h & 0xff),
		0x08, 0x06, 0x00, 0x00, 0x00,
	}
}

// mkBMP builds a minimal 54-byte BMP header with the given dimensions.
func mkBMP(t *testing.T, w, h int32) []byte {
	t.Helper()
	buf := make([]byte, 54)
	buf[0], buf[1] = 'B', 'M'
	writeLE32(buf, 14, 40)
	writeLE32(buf, 18, w)
	writeLE32(buf, 22, h)
	buf[26] = 1
	buf[28] = 24
	return buf
}

// mkJPEG builds a minimal JPEG (SOI + APP0 + SOF0) with the given dimensions.
func mkJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	buf := []byte{0xFF, 0xD8}
	buf = append(buf, 0xFF, 0xE0, 0x00, 0x10)
	buf = append(buf, make([]byte, 14)...)
	sof := []byte{
		0xFF, 0xC0, 0x00, 0x11,
		0x08,
		byte(h >> 8 & 0xff), byte(h & 0xff),
		byte(w >> 8 & 0xff), byte(w & 0xff),
	}
	return append(buf, sof...)
}

func writeLE32(b []byte, off int, v int32) {
	b[off+0] = byte(v)
	b[off+1] = byte(v >> 8)
	b[off+2] = byte(v >> 16)
	b[off+3] = byte(v >> 24)
}

// mkImageBlockBytes keeps the original signature: an image block whose source
// data is `sizeKB` of filler bytes (no format magic). Used to test the
// "unknown format → fall back to cap" path.
func mkImageBlockBytes(t *testing.T, sizeKB int) json.RawMessage {
	t.Helper()
	raw := make([]byte, sizeKB*1024)
	for i := range raw {
		raw[i] = byte(i % 251)
	}
	src := map[string]interface{}{
		"type":       "base64",
		"media_type": "image/png",
		"data":       base64.StdEncoding.EncodeToString(raw),
	}
	b, err := json.Marshal(map[string]interface{}{"type": "image", "source": src})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// mkImageBlockReal returns an image block wrapping a real-format payload.
func mkImageBlockReal(t *testing.T, payload []byte, mediaType string) json.RawMessage {
	t.Helper()
	src := map[string]interface{}{
		"type":       "base64",
		"media_type": mediaType,
		"data":       base64.StdEncoding.EncodeToString(payload),
	}
	b, err := json.Marshal(map[string]interface{}{"type": "image", "source": src})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
