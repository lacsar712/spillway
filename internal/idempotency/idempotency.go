package idempotency

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
)

var (
	ErrConflict = errors.New("idempotency key reused with a different body")
	ErrMissing  = errors.New("idempotency key not found")
)

type Record struct {
	Key        string    `json:"key"`
	BodyHash   string    `json:"body_hash"`
	CommandID  string    `json:"command_id"`
	AcceptedAt time.Time `json:"accepted_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type Store struct {
	mu    sync.Mutex
	clk   clock.Clock
	ttl   time.Duration
	byKey map[string]Record
}

func New(clk clock.Clock, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Store{
		clk:   clk,
		ttl:   ttl,
		byKey: make(map[string]Record),
	}
}

// Remember returns the original event id when the same key+hash is replayed.
// A live key with a different hash is a conflict.
func (s *Store) Remember(key, bodyHash, eventID string) (existingCommandID string, replay bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	rec, ok := s.byKey[key]
	now := s.clk.Now()
	if ok && rec.ExpiresAt.After(now) {
		if rec.BodyHash != bodyHash {
			return "", false, fmt.Errorf("%w: key=%s", ErrConflict, key)
		}
		return rec.CommandID, true, nil
	}
	s.byKey[key] = Record{
		Key:        key,
		BodyHash:   bodyHash,
		CommandID:  eventID,
		AcceptedAt: now,
		ExpiresAt:  now.Add(s.ttl),
	}
	return eventID, false, nil
}

func (s *Store) Lookup(key string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	rec, ok := s.byKey[key]
	if !ok {
		return Record{}, ErrMissing
	}
	return rec, nil
}

func (s *Store) gcLocked() {
	now := s.clk.Now()
	for k, rec := range s.byKey {
		if !rec.ExpiresAt.After(now) {
			delete(s.byKey, k)
		}
	}
}

func (s *Store) Snapshot() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	out := make([]Record, 0, len(s.byKey))
	for _, rec := range s.byKey {
		out = append(out, rec)
	}
	return out
}

func (s *Store) Restore(recs []Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey = make(map[string]Record, len(recs))
	now := s.clk.Now()
	for _, rec := range recs {
		if rec.ExpiresAt.After(now) {
			s.byKey[rec.Key] = rec
		}
	}
}

func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	return len(s.byKey)
}
