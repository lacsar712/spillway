package event

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

const MaxBody = 256 * 1024

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func Parse(body []byte) (Envelope, error) {
	if len(body) == 0 {
		return Envelope{}, fmt.Errorf("empty body")
	}
	if len(body) > MaxBody {
		return Envelope{}, fmt.Errorf("body %d exceeds %d bytes", len(body), MaxBody)
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Envelope{}, fmt.Errorf("json: %w", err)
	}
	if err := ValidateType(env.Type); err != nil {
		return Envelope{}, err
	}
	if len(env.Payload) == 0 || string(env.Payload) == "null" {
		return Envelope{}, fmt.Errorf("payload is required")
	}
	if !json.Valid(env.Payload) {
		return Envelope{}, fmt.Errorf("payload is not valid json")
	}
	return env, nil
}

func ValidateType(t string) error {
	t = strings.TrimSpace(t)
	if t == "" {
		return fmt.Errorf("event type is required")
	}
	if len(t) > 128 {
		return fmt.Errorf("event type too long")
	}
	parts := strings.Split(t, ".")
	if len(parts) < 1 {
		return fmt.Errorf("event type is empty")
	}
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("event type has empty segment")
		}
		for _, r := range p {
			if unicode.IsUpper(r) {
				return fmt.Errorf("event type must be lowercase")
			}
			ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
			if !ok {
				return fmt.Errorf("event type has illegal character %q", r)
			}
		}
	}
	return nil
}

func MatchPrefix(eventType, prefix string) bool {
	if prefix == "" {
		return true
	}
	if prefix == eventType {
		return true
	}
	if strings.HasSuffix(prefix, ".") {
		return strings.HasPrefix(eventType, prefix)
	}
	return eventType == prefix || strings.HasPrefix(eventType, prefix+".")
}
