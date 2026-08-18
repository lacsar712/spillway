package runtime

import (
	"sync"

	"github.com/lacsar712/spillway/internal/circuit"
	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/ratelimit"
)

type Limits struct {
	mu         sync.Mutex
	clk        clock.Clock
	breakerCfg circuit.Settings
	breakers   map[string]*circuit.Breaker
	buckets    map[string]*ratelimit.Bucket
	rates      map[string]rateSpec
}

type rateSpec struct {
	rate  float64
	burst int
}

func NewLimits(clk clock.Clock) *Limits {
	return NewLimitsWithCircuit(clk, circuit.DefaultSettings())
}

func NewLimitsWithCircuit(clk clock.Clock, cfg circuit.Settings) *Limits {
	return &Limits{
		clk:        clk,
		breakerCfg: cfg,
		breakers:   make(map[string]*circuit.Breaker),
		buckets:    make(map[string]*ratelimit.Bucket),
		rates:      make(map[string]rateSpec),
	}
}

func (g *Limits) Ensure(id string, rate float64, burst int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.breakers[id]; !ok {
		g.breakers[id] = circuit.New(g.clk, g.breakerCfg)
	}
	spec := rateSpec{rate: rate, burst: burst}
	old, exists := g.rates[id]
	if !exists || old != spec {
		g.buckets[id] = ratelimit.New(g.clk, rate, burst)
		g.rates[id] = spec
	}
}

func (g *Limits) Breaker(id string) *circuit.Breaker {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, ok := g.breakers[id]
	if !ok {
		b = circuit.New(g.clk, g.breakerCfg)
		g.breakers[id] = b
	}
	return b
}

func (g *Limits) Bucket(id string) *ratelimit.Bucket {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, ok := g.buckets[id]
	if !ok {
		b = ratelimit.New(g.clk, 5, 5)
		g.buckets[id] = b
	}
	return b
}

type Snapshot struct {
	Breakers map[string]circuit.Snapshot   `json:"breakers"`
	Buckets  map[string]ratelimit.Snapshot `json:"buckets"`
}

func (g *Limits) Snapshot() Snapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := Snapshot{
		Breakers: make(map[string]circuit.Snapshot, len(g.breakers)),
		Buckets:  make(map[string]ratelimit.Snapshot, len(g.buckets)),
	}
	for id, b := range g.breakers {
		s.Breakers[id] = b.Snapshot()
	}
	for id, b := range g.buckets {
		s.Buckets[id] = b.Snapshot()
	}
	return s
}

func (g *Limits) Restore(s Snapshot) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, snap := range s.Breakers {
		b := circuit.New(g.clk, g.breakerCfg)
		b.Restore(snap)
		g.breakers[id] = b
	}
	for id, snap := range s.Buckets {
		b := ratelimit.New(g.clk, snap.Rate, int(snap.Burst))
		b.Restore(snap)
		g.buckets[id] = b
	}
}

func (g *Limits) Public() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]any, len(g.breakers))
	for id, b := range g.breakers {
		out[id] = b.Snapshot()
	}
	return out
}
