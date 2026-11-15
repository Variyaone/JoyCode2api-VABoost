package joycode

import (
	"regexp"
	"strings"
)

// The upstream rejects a reasoning effort a model does not accept with a
// message like:
//
//	Unsupported value: 'none' is not supported with the 'gpt-6-astra-2026-09-03'
//	model. Supported values are: 'low', 'medium', 'high', 'xhigh', and 'max'.
//
// Measured 2026-11-15: gpt-6-astra and gpt-6.1-sol accept only
// low/medium/high/xhigh/max, while gpt-6-sol and gpt-5.6-sol also accept
// 'none'. Which models reject 'none' changes with the upstream catalog, so
// the proxy learns the answer from the error instead of hard-coding a list
// that would go stale the same way a model list does.
var unsupportedEffortRe = regexp.MustCompile(`Unsupported value: '([^']+)' is not supported with the '([^']+)' model\.`)
var effortValueRe = regexp.MustCompile(`'([^']+)'`)

// UnsupportedEffort reports whether an upstream error rejects a reasoning
// effort value. It returns the rejected value and the lowest value the model
// accepts: the upstream lists its accepted values in ascending order, so the
// first one is the closest achievable to "no reasoning".
func UnsupportedEffort(body string) (rejected string, lowest string, ok bool) {
	m := unsupportedEffortRe.FindStringSubmatch(body)
	if m == nil {
		return "", "", false
	}
	const marker = "Supported values are:"
	idx := strings.Index(body, marker)
	if idx < 0 {
		return "", "", false
	}
	segment := body[idx+len(marker):]
	// The values end where the JSON payload resumes; trailing content is
	// harmless because only the first (lowest) value is used.
	if cut := strings.IndexAny(segment, "\"\\\n"); cut >= 0 {
		segment = segment[:cut]
	}
	values := effortValueRe.FindAllStringSubmatch(segment, -1)
	if len(values) == 0 {
		return "", "", false
	}
	return m[1], values[0][1], true
}

// ThinkingDisabled reports whether a translated body's thinking control
// explicitly turns reasoning off.
func ThinkingDisabled(raw interface{}) bool {
	thinking, ok := raw.(map[string]interface{})
	if !ok {
		return false
	}
	typ, _ := thinking["type"].(string)
	return strings.EqualFold(typ, "disabled")
}
