package circuit

import (
	"sync"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

type Settings struct {
	FailThreshold int
	OpenFor       time.Duration
	Probes        int
}

func DefaultSettings() Settings {
	return Settings{
		FailThreshold: 5,
		OpenFor:       30 * time.Second,
		Probes:        1,
	}
}

func (s Settings) normalize() Settings {
	if s.FailThreshold < 1 {
		s.FailThreshold = 5
	}
	if s.OpenFor <= 0 {
		s.OpenFor = 30 * time.Second
	}
	if s.Probes < 1 {
		s.Probes = 1
	}
	return s
}

type Breaker struct {
	mu         sync.Mutex
	clk        clock.Clock
	cfg        Settings
	state      State
	failures   int
	openedAt   time.Time
	probesLeft int
}

func New(clk clock.Clock, cfg Settings) *Breaker {
	return &Breaker{clk: clk, cfg: cfg.normalize(), state: Closed}
}

type Decision struct {
	Allow bool
	State State
	Note  string
}

func (b *Breaker) Allow() Decision {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maybeHalfOpenLocked()
	switch b.state {
	case Closed:
		return Decision{Allow: true, State: Closed, Note: "closed"}
	case Open:
		return Decision{Allow: false, State: Open, Note: "open"}
	case HalfOpen:
		if b.probesLeft <= 0 {
			return Decision{Allow: false, State: HalfOpen, Note: "probe_budget_empty"}
		}
		b.probesLeft--
		return Decision{Allow: true, State: HalfOpen, Note: "probe"}
	default:
		return Decision{Allow: false, State: b.state, Note: "unknown"}
	}
}

func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	// A successful delivery clears accumulated failures so a later,
	// isolated failure does not trip the breaker on top of stale state.
	b.failures = 0
	switch b.state {
	case HalfOpen:
		// A probe succeeded: close the breaker and clear the probe budget.
		b.state = Closed
		b.probesLeft = 0
	case Open:
		// Stay open until the cooldown elapses; a stray success (e.g. a
		// racing probe) must not short-circuit the cooldown into Closed.
	case Closed:
		// failures already cleared above.
	}
}

func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case HalfOpen:
		b.tripLocked()
		return
	case Closed:
		b.failures++
		if b.failures >= b.cfg.FailThreshold {
			b.tripLocked()
		}
	case Open:
		// stay open until timer
	}
}

func (b *Breaker) tripLocked() {
	b.state = Open
	b.openedAt = b.clk.Now()
	b.probesLeft = 0
}

func (b *Breaker) maybeHalfOpenLocked() {
	if b.state != Open {
		return
	}
	if b.clk.Now().Sub(b.openedAt) >= b.cfg.OpenFor {
		b.state = HalfOpen
		b.probesLeft = b.cfg.Probes
	}
}

func (b *Breaker) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maybeHalfOpenLocked()
	return Snapshot{
		State:      b.state.String(),
		Failures:   b.failures,
		OpenedAt:   b.openedAt,
		ProbesLeft: b.probesLeft,
	}
}

type Snapshot struct {
	State      string    `json:"state"`
	Failures   int       `json:"failures"`
	OpenedAt   time.Time `json:"opened_at,omitempty"`
	ProbesLeft int       `json:"probes_left"`
}

func (b *Breaker) Restore(s Snapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch s.State {
	case "open":
		b.state = Open
	case "half_open":
		b.state = HalfOpen
	default:
		b.state = Closed
	}
	b.failures = s.Failures
	b.openedAt = s.OpenedAt
	b.probesLeft = s.ProbesLeft
}
