package event_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/lacsar712/spillway/internal/event"
)

func TestParseAndPrefix(t *testing.T) {
	env, err := event.Parse([]byte(`{"type":"gate.raise","payload":{"id":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "gate.raise" {
		t.Fatalf("type %s", env.Type)
	}
	if !event.MatchPrefix("gate.raise", "gate") {
		t.Fatal("gate should match gate.raise")
	}
	if event.MatchPrefix("gate.raise", "charge") {
		t.Fatal("charge should not match")
	}
	if _, err := event.Parse([]byte(`{"type":"Order.Paid","payload":{}}`)); err == nil {
		t.Fatal("uppercase type should fail")
	}
}

func TestMatchPrefixSegmentBoundary(t *testing.T) {
	cases := []struct {
		eventType string
		prefix    string
		want      bool
	}{
		{"gate.raise", "gate", true},
		{"gate.raise", "gate.", true},
		{"gate.raise", "gate.raise", true},
		{"gate.raise", "", true},
		{"gate.raise", "ga", false},
		{"gate.raise", "gat", false},
		{"gate.raise", "gate.r", false},
		{"gate.raise", "charge", false},
		{"gates.raise", "gate", false},
	}
	for _, tc := range cases {
		got := event.MatchPrefix(tc.eventType, tc.prefix)
		if got != tc.want {
			t.Fatalf("MatchPrefix(%q, %q)=%v want %v", tc.eventType, tc.prefix, got, tc.want)
		}
	}
}

func TestParseWrapsSyntaxError(t *testing.T) {
	_, err := event.Parse([]byte(`{`))
	if err == nil {
		t.Fatal("expected syntax error")
	}
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("want json.SyntaxError via errors.As, got %v", err)
	}
}
