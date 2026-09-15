package joycode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OpenClaw account / upstream constants.
const (
	// LLM gateway
	openClawLLMBaseURL = "https://joyme-server-prod.jd.com"
	openClawLLMPath    = "/llm-gateway/chat/completions"

	// Auth chain (same as tools/joyme/joyme-direct.js, no stored credentials):
	//   desk.agent.auth.encrypt -> local HiOffice (127.0.0.1:8988) appToken
	//   -> desk.agent.auth.getWebToken -> me_token (~24h, refreshed ahead)
	openClawAuthURL   = "https://api.m.jd.com/"
	openClawAuthAppID = "JDME_DESKTOP"
	openClawEncryptFn = "desk.agent.auth.encrypt"
	openClawWebTokFn  = "desk.agent.auth.getWebToken"
	openClawJdmeAppID = "ee"

	// meToken TTL is 86400s server-side; we refresh ahead to avoid mid-request expiry.
	openClawTokenRefreshAhead = 5 * time.Minute
	// Local 京ME desktop (HiOffice) service that hands out the appToken.
	openClawHiOfficeURL = "http://127.0.0.1:8988/hioffice?from=hio_plugin_joydesk"

	// Default tenant for the llm-gateway.
	openClawTenantCode = "CN.JD.GROUP"
)

// OpenClawModels is the static model list for the openclaw backend.
var OpenClawModels = []string{"JoyAI", "Dr.Joy"}

// IsOpenClaw reports whether this client talks to the openclaw (joyme-server) backend.
func (c *Client) IsOpenClaw() bool { return c.Provider == "openclaw" }

// openClawState holds the token lifecycle state for an openclaw client.
type openClawState struct {
	mu         sync.Mutex
	meToken    string
	meTokenExp time.Time
}

// openClawEncryptEnvelope holds the Color-gateway response of
// desk.agent.auth.encrypt: an AES-encrypted payload plus its key.
type openClawEncryptEnvelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AESKey  string `json:"aesKey"`
		Content string `json:"content"`
	} `json:"data"`
}

// openClawWebTokenResponse is the success shape of desk.agent.auth.getWebToken.
type openClawWebTokenResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken         string `json:"accessToken"`
		AccessTokenExpireIn int    `json:"accessTokenExpireIn"`
	} `json:"data"`
}

// SetOpenClawContext configures this client to talk to the openclaw backend.
// Credentials are acquired on demand from the local 京ME desktop (HiOffice);
// the openclaw backend has no per-account credentials.
func (c *Client) SetOpenClawContext() {
	c.Provider = "openclaw"
	if c.openClaw == nil {
		c.openClaw = &openClawState{}
	}
}

// openClawEnsureToken returns a valid meToken, acquiring a fresh one via the
// HiOffice chain when missing or near expiry.
func (c *Client) openClawEnsureToken() (string, error) {
	c.openClaw.mu.Lock()
	defer c.openClaw.mu.Unlock()

	st := c.openClaw
	if st.meToken != "" && time.Until(st.meTokenExp) > openClawTokenRefreshAhead {
		return st.meToken, nil
	}

	access, expiresIn, err := c.openClawWebToken()
	if err != nil {
		return "", err
	}
	st.meToken = access
	st.meTokenExp = time.Now().Add(time.Duration(expiresIn) * time.Second)
	slog.Info("openclaw me_token acquired", "expires_in", expiresIn)
	return st.meToken, nil
}

// openClawWebToken walks the credential-free auth chain (same as
// tools/joyme/joyme-direct.js):
//
//	encrypt(Color 网关) -> HiOffice(127.0.0.1:8988) appToken -> getWebToken me_token
//
// It requires the 京ME desktop app to be running locally.
func (c *Client) openClawWebToken() (accessToken string, expiresIn int, err error) {
	content := fmt.Sprintf(
		`{"method":"query","param":"appToken","timestamp":"%d","from":"hio_plugin_joydesk","to":"HiOfficeClient"}`,
		time.Now().Unix(),
	)

	// Step 1: ask the Color gateway to encrypt the request payload.
	encURL := openClawAuthURL + "?functionId=" + openClawEncryptFn + "&appid=" + openClawAuthAppID
	encBody := map[string]interface{}{
		"appid":      openClawAuthAppID,
		"functionId": openClawEncryptFn,
		"body":       map[string]interface{}{"content": content, "jdmeAppId": openClawJdmeAppID},
	}
	var enc openClawEncryptEnvelope
	if err := openClawCallJSON(encURL, encBody, &enc); err != nil {
		return "", 0, fmt.Errorf("encrypt: %w", err)
	}
	if enc.Code != 0 || enc.Data.AESKey == "" || enc.Data.Content == "" {
		return "", 0, fmt.Errorf("encrypt code=%d msg=%s", enc.Code, enc.Msg)
	}

	// Step 2: local HiOffice service returns the appToken plus its own
	// x-aes-key header for the return leg.
	req, err := http.NewRequest("POST", openClawHiOfficeURL, strings.NewReader(enc.Data.Content))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AES-Key", enc.Data.AESKey)
	hioResp, err := openClawHTTPClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("HiOffice(京ME桌面端) 不可达: %w", err)
	}
	defer hioResp.Body.Close()
	if hioResp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("HiOffice HTTP %d", hioResp.StatusCode)
	}
	appToken, err := io.ReadAll(hioResp.Body)
	if err != nil {
		return "", 0, err
	}
	hioAESKey := hioResp.Header.Get("x-aes-key")
	if len(appToken) == 0 || hioAESKey == "" {
		return "", 0, fmt.Errorf("HiOffice 空响应（京ME桌面端未登录？）")
	}

	// Step 3: exchange the appToken for a me_token.
	webURL := openClawAuthURL + "?functionId=" + openClawWebTokFn + "&appid=" + openClawAuthAppID
	webBody := map[string]interface{}{
		"appid":      openClawAuthAppID,
		"functionId": openClawWebTokFn,
		"body": map[string]interface{}{
			"token":      string(appToken),
			"tenantCode": openClawTenantCode,
			"deviceUuid": "JoyCode2Api-win",
			"aesKey":     hioAESKey,
			"jdmeAppId":  openClawJdmeAppID,
		},
	}
	var web openClawWebTokenResponse
	if err := openClawCallJSON(webURL, webBody, &web); err != nil {
		return "", 0, fmt.Errorf("getWebToken: %w", err)
	}
	if web.Code != 0 || web.Data.AccessToken == "" {
		return "", 0, fmt.Errorf("getWebToken code=%d msg=%s", web.Code, web.Msg)
	}
	return web.Data.AccessToken, web.Data.AccessTokenExpireIn, nil
}

// openClawCallJSON POSTs a Color-gateway envelope and decodes the response.
func openClawCallJSON(url string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := openClawHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return json.Unmarshal(raw, out)
}

var openClawHTTPClient = &http.Client{Timeout: 30 * time.Second}

// openClawDoStream POSTs to the LLM gateway and returns the raw response
// (which is SSE regardless of the body's "stream" flag — the gateway always streams).
func (c *Client) openClawDoStream(body map[string]interface{}) (*http.Response, error) {
	meToken, err := c.openClawEnsureToken()
	if err != nil {
		return nil, fmt.Errorf("openclaw token: %w", err)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", openClawLLMBaseURL+openClawLLMPath, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "me_token="+meToken)
	req.Header.Set("Accept", "text/event-stream")
	return c.httpClient.Do(req)
}

// openClawDo aggregates the SSE stream into a single JSON-shaped response.
// The gateway always returns SSE; we pull all chunks, concatenate the
// accumulating `content` field, and return a synthesized non-stream chat completion.
func (c *Client) openClawDo(body map[string]interface{}) (map[string]interface{}, error) {
	resp, err := c.openClawDoStream(body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openclaw HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var (
		fullText   string
		lastModel  string
		finishReason = "stop"
	)
	buf := make([]byte, 64<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			fullText += string(buf[:n])
			// Cache last seen model id from any chunk (cheap regex-free scan).
			if idx := bytes.LastIndex(buf[:n], []byte(`"model":"`)); idx >= 0 {
				rest := buf[:n][idx+9:]
				if end := bytes.IndexByte(rest, '"'); end > 0 {
					lastModel = string(rest[:end])
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	// Parse all `data: {...}` lines and concatenate delta.content.
	content := ""
	for _, line := range bytes.Split([]byte(fullText), []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[5:])
		if bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &chunk); err != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			content += ch.Delta.Content
			if ch.FinishReason != nil {
				finishReason = *ch.FinishReason
			}
		}
	}
	return map[string]interface{}{
		"id":      "chatcmpl-openclaw",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   lastModel,
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"message":       map[string]interface{}{"role": "assistant", "content": content},
				"finish_reason": finishReason,
			},
		},
	}, nil
}

// ValidateOpenClaw performs the full HiOffice -> getWebToken chain so the
// dashboard can report whether the openclaw backend is currently usable
// (requires 京ME desktop running). Returns the server's error on failure.
func (c *Client) ValidateOpenClaw() error {
	if !c.IsOpenClaw() {
		return fmt.Errorf("not an openclaw client")
	}
	_, _, err := c.openClawWebToken()
	return err
}

// openClawPost routes a chat-completions-style call.
func (c *Client) openClawPost(body map[string]interface{}) (map[string]interface{}, error) {
	return c.openClawDo(body)
}

// openClawPostStream streams the gateway's SSE untouched.
func (c *Client) openClawPostStream(body map[string]interface{}) (*http.Response, error) {
	return c.openClawDoStream(body)
}

// openClawPostAnthropicStream — the openclaw gateway does NOT have a native
// Anthropic Messages endpoint. Instead we translate: the body arriving here
// is already in chat-completions shape (the anthropic package converts
// Anthropic → chat completions upstream). Reuse the chat SSE path.
func (c *Client) openClawPostAnthropicStream(body map[string]interface{}) (*http.Response, error) {
	return c.openClawDoStream(body)
}
