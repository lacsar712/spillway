package circuit_test

import (
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/circuit"
	"github.com/lacsar712/spillway/internal/clock"
)

func TestOpensAfterThresholdAndRecovers(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(0, 0))
	b := circuit.New(clk, circuit.Settings{FailThreshold: 2, OpenFor: time.Second, Probes: 1})
	b.Failure()
	if !b.Allow().Allow {
		t.Fatal("still closed after one failure")
	}
	b.Failure()
	if b.Allow().Allow {
		t.Fatal("should be open")
	}
	clk.Advance(time.Second)
	d := b.Allow()
	if !d.Allow || d.State != circuit.HalfOpen {
		t.Fatalf("expected half-open probe, got %+v", d)
	}
	b.Success()
	if b.Allow().State != circuit.Closed {
		t.Fatal("expected closed after probe success")
	}
}

func TestSuccessClearsFailureCount(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(0, 0))
	b := circuit.New(clk, circuit.Settings{FailThreshold: 2, OpenFor: time.Hour, Probes: 1})
	b.Failure()
	b.Success()
	b.Failure()
	if !b.Allow().Allow || b.Allow().State != circuit.Closed {
		t.Fatal("a failure after success must not open the breaker")
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(0, 0))
	b := circuit.New(clk, circuit.Settings{FailThreshold: 1, OpenFor: time.Second, Probes: 1})
	b.Failure()
	if b.Allow().Allow {
		t.Fatal("should be open")
	}
	clk.Advance(time.Second)
	d := b.Allow()
	if !d.Allow || d.State != circuit.HalfOpen {
		t.Fatalf("expected half-open probe, got %+v", d)
	}
	b.Failure()
	if b.Allow().Allow {
		t.Fatal("failed probe must open the breaker again")
	}
}
