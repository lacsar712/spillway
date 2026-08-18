package backoff_test

import (
	"testing"

	"github.com/lacsar712/spillway/internal/backoff"
)

func TestExhaustedAtMaxAttempts(t *testing.T) {
	p := backoff.Policy{MaxAttempts: 8, Base: 1, Cap: 2}
	if p.Exhausted(7) {
		t.Fatal("attempt 7 of 8 must still retry")
	}
	if !p.Exhausted(8) {
		t.Fatal("attempt 8 of 8 must be exhausted")
	}
	if !p.Exhausted(9) {
		t.Fatal("attempt 9 must be exhausted")
	}
}
