package journal

import (
	"sync"
	"time"

	"github.com/lacsar712/spillway/internal/redact"
)

type Entry struct {
	At           time.Time `json:"at"`
	CommandID    string    `json:"command_id"`
	DeliveryID   string    `json:"delivery_id"`
	PLCID        string    `json:"plc_id"`
	Attempt      int       `json:"attempt"`
	Kind         string    `json:"kind"`
	Status       int       `json:"status,omitempty"`
	Error        string    `json:"error,omitempty"`
	Note         string    `json:"note,omitempty"`
	Type         string    `json:"type"`
	BodyRedacted []byte    `json:"body_redacted,omitempty"`
	Body         []byte    `json:"body,omitempty"`
	ReplayOf     string    `json:"replay_of,omitempty"`
}

type Log struct {
	mu      sync.Mutex
	max     int
	entries []Entry
}

func New(max int) *Log {
	if max < 50 {
		max = 200
	}
	return &Log{max: max, entries: make([]Entry, 0, 64)}
}

func (l *Log) Append(e Entry) {
	if len(e.Body) > 0 && len(e.BodyRedacted) == 0 {
		e.BodyRedacted = redact.JSON(e.Body)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	if len(l.entries) > l.max {
		l.entries = append([]Entry(nil), l.entries[len(l.entries)-l.max:]...)
	}
}

func (l *Log) List(destID string, limit int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]Entry, 0, limit)
	for i := len(l.entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := l.entries[i]
		if destID != "" && e.PLCID != destID {
			continue
		}
		cp := e
		cp.Body = nil // UI sees redacted only
		out = append(out, cp)
	}
	return out
}

func (l *Log) Get(deliveryID string) (Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.entries) - 1; i >= 0; i-- {
		if l.entries[i].DeliveryID == deliveryID {
			return l.entries[i], true
		}
	}
	return Entry{}, false
}

func (l *Log) Snapshot() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

func (l *Log) Restore(entries []Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(entries) > l.max {
		entries = entries[len(entries)-l.max:]
	}
	l.entries = append([]Entry(nil), entries...)
}
