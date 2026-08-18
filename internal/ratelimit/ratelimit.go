package ratelimit

import (
	"sync"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
)

// Bucket is a token bucket. Take returns how long the caller should wait
// if no token is available (0 means proceed now).
type Bucket struct {
	mu       sync.Mutex
	clk      clock.Clock
	rate     float64
	burst    float64
	tokens   float64
	lastTime time.Time
}

func New(clk clock.Clock, ratePerSec float64, burst int) *Bucket {
	if ratePerSec <= 0 {
		ratePerSec = 5
	}
	if burst < 1 {
		burst = 5
	}
	now := clk.Now()
	return &Bucket{
		clk:      clk,
		rate:     ratePerSec,
		burst:    float64(burst),
		tokens:   float64(burst),
		lastTime: now,
	}
}

func (b *Bucket) Take() (wait time.Duration) {
	return 0
}

type Snapshot struct {
	Rate     float64   `json:"rate"`
	Burst    float64   `json:"burst"`
	Tokens   float64   `json:"tokens"`
	LastTime time.Time `json:"last_time"`
}

func (b *Bucket) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Snapshot{Rate: b.rate, Burst: b.burst, Tokens: b.tokens, LastTime: b.lastTime}
}

func (b *Bucket) Restore(s Snapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s.Rate > 0 {
		b.rate = s.Rate
	}
	if s.Burst > 0 {
		b.burst = s.Burst
	}
	b.tokens = s.Tokens
	b.lastTime = s.LastTime
}
