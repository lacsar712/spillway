package interlock_test

import (
	"errors"
	"testing"

	"github.com/lacsar712/spillway/internal/event"
	"github.com/lacsar712/spillway/internal/interlock"
	"github.com/lacsar712/spillway/internal/reservoir"
)

func TestAllowRaiseWhenPoolAboveCrest(t *testing.T) {
	g := interlock.Permissive()
	env, err := event.Parse([]byte(`{"type":"gate.raise","payload":{"bay":"S1","opening_m":1.5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Allow(env); err != nil {
		t.Fatal(err)
	}
}

func TestAllowRejectsRaiseBelowCrest(t *testing.T) {
	r := reservoir.Default()
	r.LevelM = 90
	g := interlock.New(reservoir.New(r), nil)
	env, err := event.Parse([]byte(`{"type":"gate.raise","payload":{"bay":"S1","opening_m":1.5}}`))
	if err != nil {
		t.Fatal(err)
	}
	err = g.Allow(env)
	if !errors.Is(err, interlock.ErrLowPool) {
		t.Fatalf("want ErrLowPool, got %v", err)
	}
}

func TestAllowRejectsLowerInFlood(t *testing.T) {
	r := reservoir.Default()
	r.LevelM = 115
	g := interlock.New(reservoir.New(r), nil)
	env, err := event.Parse([]byte(`{"type":"gate.lower","payload":{"bay":"S1","opening_m":0}}`))
	if err != nil {
		t.Fatal(err)
	}
	err = g.Allow(env)
	if !errors.Is(err, interlock.ErrFloodClose) {
		t.Fatalf("want ErrFloodClose, got %v", err)
	}
}
