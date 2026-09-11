package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibe-coding-labs/JoyCode2Api/pkg/auth"
)

func TestUsageActivityEndpoint(t *testing.T) {
	h, s := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/usage-activity?from=2024-02-28&through=2024-03-01", 200},
		{"GET", "/api/usage-activity?from=2024-01-01&through=2024-01-31", 200},
		{"OPTIONS", "/api/usage-activity", 204},
		{"POST", "/api/usage-activity?from=2024-02-28&through=2024-03-01", 405},
		{"GET", "/api/usage-activity", 400},
		{"GET", "/api/usage-activity?from=2024-02-30&through=2024-03-01", 400},
		{"GET", "/api/usage-activity?from=2024-03-02&through=2024-03-01", 400},
		{"GET", "/api/usage-activity?from=2024-01-01&through=2024-02-01", 400},
		{"GET", "/api/usage-activity?from=2099-01-01&through=2099-01-02", 400},
		{"GET", "/api/usage-activity?from=2024-01-01&from=2024-01-02&through=2024-01-03", 400},
		{"GET", "/api/usage-activity?from=2024-01-01%27%20OR%201=1--&through=2024-01-02", 400},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if w.Code == 200 {
				m := decodeJSON(t, w)
				if m["source"] != "request_logs" || m["today"] == "" || m["timezone"] == "" || m["generated_at"] == "" {
					t.Fatal(m)
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("missing no-store")
				}
			}
		})
	}
	s.SetSetting("auth_password_hash", "configured")
	w := httptest.NewRecorder()
	auth.JWTMiddleware(s, mux).ServeHTTP(w, httptest.NewRequest("GET", "/api/usage-activity?from=2024-01-01&through=2024-01-02", nil))
	if w.Code != 401 {
		t.Fatal("activity API must require dashboard auth", w.Code)
	}
}
func TestUsageActivityUnavailableStore(t *testing.T) {
	h, _ := setupTestHandler(t)
	h.store = nil
	w := httptest.NewRecorder()
	h.handleUsageActivity(w, httptest.NewRequest("GET", "/api/usage-activity?from=2024-01-01&through=2024-01-02", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
