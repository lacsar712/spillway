package idempotency_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/idempotency"
)

func TestRememberReplayAndConflict(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(0, 0))
	s := idempotency.New(clk, time.Hour)
	id, replay, err := s.Remember("key-aaaa", "hash1", "evt1")
	if err != nil || replay || id != "evt1" {
		t.Fatalf("first: %s replay=%v err=%v", id, replay, err)
	}
	id, replay, err = s.Remember("key-aaaa", "hash1", "evt2")
	if err != nil || !replay || id != "evt1" {
		t.Fatalf("replay: %s replay=%v err=%v", id, replay, err)
	}
	_, _, err = s.Remember("key-aaaa", "hash2", "evt3")
	if !errors.Is(err, idempotency.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	clk.Advance(2 * time.Hour)
	id, replay, err = s.Remember("key-aaaa", "hash2", "evt4")
	if err != nil || replay || id != "evt4" {
		t.Fatalf("after ttl: %s replay=%v err=%v", id, replay, err)
	}
}
