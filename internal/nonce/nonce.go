package nonce

import (
	"fmt"
	"sync"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
)

type Record struct {
	Nonce     string    `json:"nonce"`
	SeenAt    time.Time `json:"seen_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Book struct {
	mu      sync.Mutex
	clk     clock.Clock
	window  time.Duration
	entries map[string]Record
}

func New(clk clock.Clock, window time.Duration) *Book {
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &Book{
		clk:     clk,
		window:  window,
		entries: make(map[string]Record),
	}
}

func (b *Book) CheckAndRemember(nonce string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gcLocked()
	if rec, ok := b.entries[nonce]; ok {
		if rec.ExpiresAt.After(b.clk.Now()) {
			return fmt.Errorf("nonce already used")
		}
	}
	now := b.clk.Now()
	b.entries[nonce] = Record{
		Nonce:     nonce,
		SeenAt:    now,
		ExpiresAt: now.Add(b.window),
	}
	return nil
}

func (b *Book) gcLocked() {
	now := b.clk.Now()
	for k, rec := range b.entries {
		if !rec.ExpiresAt.After(now) {
			delete(b.entries, k)
		}
	}
}

func (b *Book) Snapshot() []Record {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gcLocked()
	out := make([]Record, 0, len(b.entries))
	for _, rec := range b.entries {
		out = append(out, rec)
	}
	return out
}

func (b *Book) Restore(recs []Record) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = make(map[string]Record, len(recs))
	now := b.clk.Now()
	for _, rec := range recs {
		if rec.ExpiresAt.After(now) {
			b.entries[rec.Nonce] = rec
		}
	}
}

func (b *Book) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gcLocked()
	return len(b.entries)
}
