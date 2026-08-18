package ratelimit_test

import (
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/ratelimit"
)

func TestTakeBlocksAfterBurst(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(0, 0))
	b := ratelimit.New(clk, 0.001, 1)
	if wait := b.Take(); wait != 0 {
		t.Fatalf("first take must proceed, wait=%s", wait)
	}
	if wait := b.Take(); wait == 0 {
		t.Fatal("burst exhausted; second take must return a wait")
	}
}
