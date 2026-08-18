package redact

import (
	"encoding/json"
	"strings"
)

var sensitiveKeys = []string{
	"authorization",
	"password",
	"secret",
	"token",
	"api_key",
	"apikey",
	"access_key",
	"private_key",
}

const mask = "***"

func isSensitive(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	for _, s := range sensitiveKeys {
		if k == s || strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// JSON returns a copy of the document with sensitive object keys masked.
// Invalid JSON is returned unchanged as a string.
func JSON(raw []byte) json.RawMessage {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage(raw)
	}
	masked := walk(v)
	out, err := json.Marshal(masked)
	if err != nil {
		return json.RawMessage(raw)
	}
	return out
}

func walk(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			if isSensitive(k) {
				out[k] = mask
				continue
			}
			out[k] = walk(child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = walk(child)
		}
		return out
	default:
		return t
	}
}

func Headers(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if isSensitive(k) {
			out[k] = mask
			continue
		}
		out[k] = v
	}
	return out
}
