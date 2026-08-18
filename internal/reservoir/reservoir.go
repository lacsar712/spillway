package reservoir

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Reading is a hydrologic snapshot used by the gate interlock.
type Reading struct {
	LevelM     float64   `json:"level_m"`
	CrestM     float64   `json:"crest_m"`
	FloodM     float64   `json:"flood_m"`
	MinPoolM   float64   `json:"min_pool_m"`
	TailwaterM float64   `json:"tailwater_m"`
	InflowCMS  float64   `json:"inflow_cms"`
	OutflowCMS float64   `json:"outflow_cms"`
	StorageMCM float64   `json:"storage_mcm"`
	TakenAt    time.Time `json:"taken_at"`
	Station    string    `json:"station"`
}

func Default() Reading {
	return Reading{
		LevelM:     108.4,
		CrestM:     100.0,
		FloodM:     115.0,
		MinPoolM:   95.0,
		TailwaterM: 42.0,
		InflowCMS:  320,
		OutflowCMS: 180,
		StorageMCM: 1.42e3,
		TakenAt:    time.Unix(1_700_000_000, 0).UTC(),
		Station:    "forebay-1",
	}
}

func (r Reading) HeadM() float64 {
	h := r.LevelM - r.CrestM
	if h < 0 {
		return 0
	}
	return h
}

func (r Reading) FreeboardM() float64 {
	return r.FloodM - r.LevelM
}

func (r Reading) SurplusCMS() float64 {
	return r.InflowCMS - r.OutflowCMS
}

func (r Reading) Validate() error {
	if r.CrestM <= 0 || r.FloodM <= r.CrestM {
		return fmt.Errorf("invalid dam elevations crest=%.2f flood=%.2f", r.CrestM, r.FloodM)
	}
	if r.MinPoolM <= 0 || r.MinPoolM >= r.CrestM {
		return fmt.Errorf("invalid min pool %.2f vs crest %.2f", r.MinPoolM, r.CrestM)
	}
	if r.LevelM < 0 || r.LevelM > r.FloodM+20 {
		return fmt.Errorf("implausible reservoir level %.2f m", r.LevelM)
	}
	if r.InflowCMS < 0 || r.OutflowCMS < 0 {
		return fmt.Errorf("negative flow is not physical")
	}
	return nil
}

// Pool is the process-local reservoir state. Operators can update the
// reading; the command path reads it under the same lock.
type Pool struct {
	mu sync.Mutex
	r  Reading
}

func New(r Reading) *Pool {
	if err := r.Validate(); err != nil {
		r = Default()
	}
	if r.Station == "" {
		r.Station = "forebay-1"
	}
	if r.TakenAt.IsZero() {
		r.TakenAt = time.Now().UTC()
	}
	r.StorageMCM = StorageFromLevel(r.LevelM, r.CrestM)
	return &Pool{r: r}
}

func (p *Pool) Snapshot() Reading {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r
}

func (p *Pool) Set(r Reading) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Station == "" {
		r.Station = "forebay-1"
	}
	if r.TakenAt.IsZero() {
		r.TakenAt = time.Now().UTC()
	}
	r.StorageMCM = StorageFromLevel(r.LevelM, r.CrestM)
	p.mu.Lock()
	p.r = r
	p.mu.Unlock()
	return nil
}

func (p *Pool) LevelM() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r.LevelM
}

func (p *Pool) CrestM() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r.CrestM
}

func (p *Pool) FloodM() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r.FloodM
}

func (p *Pool) MinPoolM() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r.MinPoolM
}

func (p *Pool) TailwaterM() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r.TailwaterM
}

func (p *Pool) InflowCMS() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.r.InflowCMS
}

func (p *Pool) Restore(r Reading) {
	if err := r.Validate(); err != nil {
		return
	}
	p.mu.Lock()
	p.r = r
	p.mu.Unlock()
}

// StorageFromLevel is a piecewise-linear area-capacity curve used when
// telemetry does not publish storage directly.
func StorageFromLevel(levelM, crestM float64) float64 {
	if levelM <= 0 {
		return 0
	}
	dead := math.Max(crestM-40, 20)
	if levelM <= dead {
		return 40 + 2.1*(levelM)
	}
	head := levelM - dead
	return 120 + 18*head + 0.12*head*head
}

// RiseHours estimates hours until flood stage at the current surplus.
func RiseHours(r Reading) float64 {
	surplus := r.SurplusCMS()
	if surplus <= 1 {
		return math.Inf(1)
	}
	free := math.Max(r.FreeboardM(), 0)
	areaKM2 := 12.0 + 0.08*math.Max(r.LevelM-r.MinPoolM, 0)
	volM3 := free * areaKM2 * 1e6
	return volM3 / (surplus * 3600)
}
