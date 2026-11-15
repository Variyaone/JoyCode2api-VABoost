package joycode

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Upstream model catalog.
//
// The upstream chat endpoint only accepts the id published as "chatApiModel"
// by joycode_modelList. The JD- prefixed entries are display labels: sending
// "JD-GLM-5.3" upstream returns code 6001 ("当前模型不可用或已下线"),
// while its chatApiModel "GLM-5.3-jcloud" serves the request normally.
// The JoyCode IDE resolves label -> id client-side before calling; this
// package does the same for the proxy so both spellings reach the same
// deployment.
//
// The catalog is cached process-wide and refreshed from the upstream model
// list, so a newly published deployment does not need a code change to be
// usable.

const (
	// catalogTTL bounds how long a cached catalog is trusted.
	catalogTTL = 30 * time.Minute
	// catalogFetchTimeout keeps a slow upstream from stalling a request.
	catalogFetchTimeout = 15 * time.Second
)

// Catalog maps every published identifier to the id the upstream accepts.
type Catalog struct {
	// upstream maps label / modelId / chatApiModel -> chatApiModel.
	upstream map[string]string
	// labels maps chatApiModel -> display label.
	labels map[string]string
	// entries keeps the upstream model list as published, in upstream order.
	entries []ModelInfo
	// fetchedAt is when the catalog was built.
	fetchedAt time.Time
}

var (
	catalogMu sync.RWMutex
	catalog   *Catalog

	catalogRefreshMu  sync.Mutex
	catalogRefreshing bool
)

// currentCatalog returns the cached catalog, or nil when none was fetched.
func currentCatalog() *Catalog {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	return catalog
}

// ResolveUpstream returns the id the upstream endpoint accepts for a
// client-facing model name. Names the upstream does not publish are returned
// unchanged, so a catalog that could not be fetched never breaks a request.
func ResolveUpstream(name string) string {
	if name == "" {
		return name
	}
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	if catalog == nil {
		return name
	}
	if id, ok := catalog.upstream[name]; ok {
		return id
	}
	return name
}

// KnownModel reports whether name refers to a model the upstream publishes.
// It covers the built-in list plus the live upstream catalog, so an id that
// upstream just published is not silently rewritten to the account default.
func KnownModel(name string) bool {
	if name == "" {
		return false
	}
	if IsCompletionOnlyModel(name) {
		return true
	}
	for _, m := range Models {
		if m == name {
			return true
		}
	}
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	if catalog == nil {
		return false
	}
	_, ok := catalog.upstream[name]
	return ok
}

// CatalogNames returns the display labels the upstream currently publishes,
// sorted for stable display. It is empty until the catalog is first fetched.
func CatalogNames() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	if catalog == nil {
		return nil
	}
	names := make([]string, 0, len(catalog.labels))
	for _, label := range catalog.labels {
		names = append(names, label)
	}
	return names
}

// CatalogEntries returns the upstream model list as published: every model
// with its display label and the id the upstream accepts for it, in upstream
// order. It is empty until the catalog is first fetched.
func CatalogEntries() []ModelInfo {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	if catalog == nil {
		return nil
	}
	out := make([]ModelInfo, len(catalog.entries))
	copy(out, catalog.entries)
	return out
}

// CatalogFetchedAt reports when the cached catalog was built.
func CatalogFetchedAt() (time.Time, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	if catalog == nil {
		return time.Time{}, false
	}
	return catalog.fetchedAt, true
}

// InstallCatalog installs an upstream model list as the process-wide catalog.
// An empty list clears the catalog. Callers that already hold a freshly
// fetched model list use it to avoid a second upstream call.
func InstallCatalog(models []ModelInfo) {
	if len(models) == 0 {
		catalogMu.Lock()
		catalog = nil
		catalogMu.Unlock()
		return
	}
	next := &Catalog{
		upstream:  make(map[string]string, len(models)*3),
		labels:    make(map[string]string, len(models)),
		entries:   make([]ModelInfo, 0, len(models)),
		fetchedAt: time.Now(),
	}
	for _, m := range models {
		id := strings.TrimSpace(m.ChatAPIModel)
		if id == "" {
			continue
		}
		next.upstream[id] = id
		if label := strings.TrimSpace(m.Label); label != "" {
			next.upstream[label] = id
			next.labels[id] = label
		}
		if modelID := strings.TrimSpace(m.ModelID); modelID != "" {
			next.upstream[modelID] = id
		}
		next.entries = append(next.entries, m)
	}
	catalogMu.Lock()
	catalog = next
	catalogMu.Unlock()
}

// EnsureCatalog loads the upstream model catalog when it is missing or
// stale. The first request after startup blocks on one modelList call;
// later refreshes run in the background so a request never waits on the
// catalog.
func (c *Client) EnsureCatalog() {
	if c == nil || c.IsOpenClaw() {
		return
	}
	cat := currentCatalog()
	if cat != nil && time.Since(cat.fetchedAt) < catalogTTL {
		return
	}
	if cat != nil {
		go c.refreshCatalog()
		return
	}
	c.refreshCatalog()
}

// refreshCatalog fetches the upstream model list and replaces the cache.
// Concurrent callers collapse into one upstream call.
func (c *Client) refreshCatalog() {
	catalogRefreshMu.Lock()
	if catalogRefreshing {
		catalogRefreshMu.Unlock()
		return
	}
	catalogRefreshing = true
	catalogRefreshMu.Unlock()
	defer func() {
		catalogRefreshMu.Lock()
		catalogRefreshing = false
		catalogRefreshMu.Unlock()
	}()

	// The shared client allows a 30 minute request timeout; a catalog fetch
	// that hangs would hold up the request that triggered it.
	sc := *c
	sc.httpClient = &http.Client{Timeout: catalogFetchTimeout, Transport: defaultTransport}

	models, err := sc.ListModels()
	if err != nil {
		slog.Warn("upstream model catalog refresh failed", "error", err)
		return
	}
	if len(models) == 0 {
		slog.Warn("upstream model catalog empty, keeping previous catalog")
		return
	}
	InstallCatalog(models)
	slog.Info("upstream model catalog refreshed", "models", len(models))
}
