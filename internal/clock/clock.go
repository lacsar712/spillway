package clock

import "time"

// Clock is injected so signature windows, TTL, backoff and circuit
// recovery can be tested without sleeping on the wall clock.
type Clock interface {
	Now() time.Time
}

type Real struct{}

func (Real) Now() time.Time { return time.Now() }

// Frozen returns a constant instant until Advance is called.
type Frozen struct {
	t time.Time
}

func NewFrozen(t time.Time) *Frozen { return &Frozen{t: t} }

func (f *Frozen) Now() time.Time { return f.t }

func (f *Frozen) Advance(d time.Duration) { f.t = f.t.Add(d) }

func (f *Frozen) Set(t time.Time) { f.t = t }
