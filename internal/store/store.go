package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lacsar712/spillway/internal/dlq"
	"github.com/lacsar712/spillway/internal/idempotency"
	"github.com/lacsar712/spillway/internal/ingest"
	"github.com/lacsar712/spillway/internal/job"
	"github.com/lacsar712/spillway/internal/journal"
	"github.com/lacsar712/spillway/internal/nonce"
	"github.com/lacsar712/spillway/internal/plant"
	"github.com/lacsar712/spillway/internal/queue"
	"github.com/lacsar712/spillway/internal/runtime"
)

type Snapshot struct {
	SavedAt     time.Time            `json:"saved_at"`
	PLCs        []plant.PLC          `json:"plcs"`
	Idempotency []idempotency.Record `json:"idempotency"`
	Nonces      []nonce.Record       `json:"nonces"`
	Journal     []journal.Entry      `json:"journal"`
	DLQ         []dlq.Item           `json:"dlq"`
	Jobs        []job.Job            `json:"jobs"`
	IngestKeys  []ingest.Key         `json:"ingest_keys"`
	Limits      runtime.Snapshot     `json:"gates"`
}

type File struct {
	mu   sync.Mutex
	path string
}

func New(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &File{path: filepath.Join(dir, "snapshot.json")}, nil
}

func (f *File) Save(s Snapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s.SavedAt = time.Now().UTC()
	tmp := f.path + ".tmp"
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *File) Load() (Snapshot, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return Snapshot{}, false, err
	}
	return s, true, nil
}

type Deps struct {
	Plants *plant.Registry
	Idem   *idempotency.Store
	Nonces *nonce.Book
	Log    *journal.Log
	Dead   *dlq.Queue
	Broker *queue.Broker
	Keys   *ingest.Keys
	Limits *runtime.Limits
}

func Capture(d Deps) Snapshot {
	meta := map[string]queue.DestMeta{}
	for _, dest := range d.Plants.List() {
		meta[dest.ID] = queue.DestMeta{Ordered: dest.Ordered, MaxInFlight: dest.MaxInFlight}
	}
	_ = meta
	return Snapshot{
		PLCs:        d.Plants.List(),
		Idempotency: d.Idem.Snapshot(),
		Nonces:      d.Nonces.Snapshot(),
		Journal:     d.Log.Snapshot(),
		DLQ:         d.Dead.Snapshot(),
		Jobs:        d.Broker.Snapshot(),
		IngestKeys:  d.Keys.Snapshot(),
		Limits:      d.Limits.Snapshot(),
	}
}

func Apply(d Deps, s Snapshot) {
	if len(s.PLCs) > 0 {
		d.Plants.Restore(s.PLCs)
	}
	d.Idem.Restore(s.Idempotency)
	d.Nonces.Restore(s.Nonces)
	d.Log.Restore(s.Journal)
	d.Dead.Restore(s.DLQ)
	meta := map[string]queue.DestMeta{}
	for _, dest := range s.PLCs {
		meta[dest.ID] = queue.DestMeta{Ordered: dest.Ordered, MaxInFlight: dest.MaxInFlight}
	}
	d.Broker.Restore(s.Jobs, meta)
	if len(s.IngestKeys) > 0 {
		d.Keys.Restore(s.IngestKeys)
	}
	d.Limits.Restore(s.Limits)
}
