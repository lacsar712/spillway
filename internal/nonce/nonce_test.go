package nonce_test

import (
	"testing"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/nonce"
)

func TestCheckAndRememberRejectsReuse(t *testing.T) {
	clk := clock.NewFrozen(time.Unix(1_700_000_000, 0))
	b := nonce.New(clk, 5*time.Minute)
	if err := b.CheckAndRemember("abcdefghijklmnop"); err != nil {
		t.Fatal(err)
	}
	if err := b.CheckAndRemember("abcdefghijklmnop"); err == nil {
		t.Fatal("reused nonce must be rejected")
	}
	if b.Len() != 1 {
		t.Fatalf("book len=%d want 1", b.Len())
	}
}
