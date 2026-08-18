package loopback

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type Received struct {
	At         time.Time         `json:"at"`
	Headers    map[string]string `json:"headers"`
	Body       json.RawMessage   `json:"body"`
	CommandID  string            `json:"command_id"`
	DeliveryID string            `json:"delivery_id"`
	Attempt    string            `json:"attempt"`
	PLCID      string            `json:"plc_id"`
}

type Sink struct {
	mu    sync.Mutex
	max   int
	items []Received
}

func New(max int) *Sink {
	if max < 10 {
		max = 50
	}
	return &Sink{max: max}
}

func (s *Sink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	rec := Received{
		At:         time.Now(),
		Headers:    map[string]string{},
		Body:       json.RawMessage(body),
		CommandID:  r.Header.Get("X-Spill-Command-Id"),
		DeliveryID: r.Header.Get("X-Spill-Delivery-Id"),
		Attempt:    r.Header.Get("X-Spill-Attempt"),
		PLCID:      r.Header.Get("X-Spill-PLC"),
	}
	keep := []string{
		"Content-Type",
		"X-Spill-Timestamp",
		"X-Spill-Nonce",
		"X-Spill-Signature",
		"X-Spill-Command-Id",
		"X-Spill-Delivery-Id",
		"X-Spill-Attempt",
		"X-Spill-PLC",
		"X-Spill-Body-Sha256",
	}
	for _, k := range keep {
		if v := r.Header.Get(k); v != "" {
			rec.Headers[k] = v
		}
	}
	s.push(rec)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Sink) push(rec Received) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, rec)
	if len(s.items) > s.max {
		s.items = append([]Received(nil), s.items[len(s.items)-s.max:]...)
	}
}

func (s *Sink) List() []Received {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Received, len(s.items))
	for i := range s.items {
		out[i] = s.items[len(s.items)-1-i]
	}
	return out
}
