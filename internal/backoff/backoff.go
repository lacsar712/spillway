package backoff

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"time"
)

type Policy struct {
	Base        time.Duration
	Cap         time.Duration
	MaxAttempts int
}

func Default() Policy {
	return Policy{
		Base:        200 * time.Millisecond,
		Cap:         30 * time.Second,
		MaxAttempts: 8,
	}
}

func (p Policy) Validate() Policy {
	if p.Base <= 0 {
		p.Base = 200 * time.Millisecond
	}
	if p.Cap < p.Base {
		p.Cap = 30 * time.Second
	}
	if p.MaxAttempts < 1 {
		p.MaxAttempts = 8
	}
	return p
}

func (p Policy) Exhausted(attempt int) bool {
	p = p.Validate()
	return attempt >= p.MaxAttempts
}

// Delay implements full jitter: random in [0, min(cap, base*2^(attempt-1))].
// attempt is 1-based (the attempt that just failed).
func (p Policy) Delay(attempt int) time.Duration {
	p = p.Validate()
	if attempt < 1 {
		attempt = 1
	}
	exp := attempt - 1
	if exp > 30 {
		exp = 30
	}
	max := float64(p.Base) * math.Pow(2, float64(exp))
	if max > float64(p.Cap) {
		max = float64(p.Cap)
	}
	if max <= 0 {
		return 0
	}
	return time.Duration(randFloat64() * max)
}

func randFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0.5
	}
	// 53 bits of mantissa
	n := binary.BigEndian.Uint64(buf[:]) >> 11
	return float64(n) / (1 << 53)
}

// DelayDeterministic is used in tests: midpoint of the jitter window.
func (p Policy) DelayDeterministic(attempt int) time.Duration {
	p = p.Validate()
	if attempt < 1 {
		attempt = 1
	}
	exp := attempt - 1
	if exp > 30 {
		exp = 30
	}
	max := float64(p.Base) * math.Pow(2, float64(exp))
	if max > float64(p.Cap) {
		max = float64(p.Cap)
	}
	return time.Duration(max / 2)
}
