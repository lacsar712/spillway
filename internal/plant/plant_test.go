package plant_test

import (
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/plant"
)

func TestMatchesSegmentBoundary(t *testing.T) {
	cases := []struct {
		prefix    string
		eventType string
		want      bool
	}{
		{"gate", "gate.raise", true},
		{"gate.", "gate.raise", true},
		{"gate.raise", "gate.raise", true},
		{"", "anything.else", true},
		{"ga", "gate.raise", false},
		{"gat", "gate.raise", false},
		{"gate.r", "gate.raise", false},
		{"charge", "gate.raise", false},
		{"gate", "gates.raise", false},
	}
	for _, tc := range cases {
		d := plant.PLC{
			Enabled:      true,
			TypePrefixes: []string{tc.prefix},
		}
		got := d.Matches(tc.eventType)
		if got != tc.want {
			t.Fatalf("prefix %q vs type %q: Matches=%v want %v", tc.prefix, tc.eventType, got, tc.want)
		}
	}
}

func TestMatchesDisabledNeverFires(t *testing.T) {
	d := plant.PLC{
		Enabled:      false,
		TypePrefixes: []string{""},
	}
	if d.Matches("gate.raise") {
		t.Fatal("disabled destination must not match")
	}
}

func TestRegistryMatchingSkipsDisabled(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(0, 0))
	reg := plant.NewRegistry(clk)
	off := false
	d, err := reg.Create(plant.CreateInput{
		Name:         "off",
		URL:          "http://127.0.0.1:8080/api/v1/loopback",
		Secret:       "abcdefgh",
		TypePrefixes: []string{"gate"},
		Enabled:      &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Enabled {
		t.Fatal("expected disabled")
	}
	got := reg.Matching("gate.raise")
	if len(got) != 0 {
		t.Fatalf("disabled dest leaked into matching: %+v", got)
	}
}
