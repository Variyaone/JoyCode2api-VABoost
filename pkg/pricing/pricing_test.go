package pricing

import (
	"net/url"
	"testing"
)

func TestRatesAndAliases(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rates {
		if seen[r.Model] {
			t.Fatal("duplicate model")
		}
		seen[r.Model] = true
		if (r.Input == nil) != (r.Output == nil) {
			t.Fatal("incomplete rate")
		}
		if r.Input != nil {
			u, e := url.Parse(r.URL)
			if e != nil || u.Scheme != "https" || r.Source == "" || *r.Input < 0 || *r.Output < 0 {
				t.Fatal("invalid provenance/rate")
			}
		}
	}
	if Lookup("GPT-unknown").Input != nil {
		t.Fatal("unknown prefix was priced")
	}
	if Lookup("Kimi-K3-jcloud").Input == nil {
		t.Fatal("jcloud model must inherit vendor list price")
	}
	if *Lookup("Claude-Opus-5-hq").Input != 50 {
		t.Fatal("explicit alias missing")
	}
}

// cny() stores tenths of micro-USD; the dashboard renders USD as
// stored/10 and CNY as stored/10*7.2. Converting back must recover the
// official CNY list price to within rounding (<1% was the 10x bug).
func TestCNYRoundTrip(t *testing.T) {
	cases := map[string][2]int64{
		"GLM-5.3":            {8, 28},
		"GLM-5.2-jcloud":     {8, 28},
		"Kimi-K3":            {20, 100},
		"Kimi-K3-jcloud":     {20, 100},
		"Doubao-Seed-2.0-pro": {48, 240},
	}
	for _, r := range Rates {
		want, ok := cases[r.Model]
		if !ok {
			continue
		}
		if r.Input == nil || r.Output == nil {
			t.Fatalf("%s: missing rate", r.Model)
		}
		for i, got := range [2]int64{*r.Input, *r.Output} {
			back := float64(got) / 10 * 7.2 // tenths of micro-USD -> CNY per 1M
			if d := back - float64(want[i]); d < -1 || d > 1 {
				t.Errorf("%s %s: stored %d renders ¥%.2f, official ¥%d", r.Model, []string{"input", "output"}[i], got, back, want[i])
			}
		}
	}
}
