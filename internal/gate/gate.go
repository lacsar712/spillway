package gate

import (
	"fmt"
	"strings"
	"sync"
)

// Bay is one radial or tainter gate on the spillway.
type Bay struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	WidthM      float64 `json:"width_m"`
	MaxOpenM    float64 `json:"max_open_m"`
	MinOpenM    float64 `json:"min_open_m"`
	TravelMPerS float64 `json:"travel_m_per_s"`
	PositionM   float64 `json:"position_m"`
	Enabled     bool    `json:"enabled"`
	PLCBayReg   int     `json:"plc_bay_reg"`
}

func DefaultCatalog() []Bay {
	return []Bay{
		{ID: "S1", Name: "spill bay 1", Kind: "tainter", WidthM: 12, MaxOpenM: 8, MinOpenM: 0, TravelMPerS: 0.04, PositionM: 0.5, Enabled: true, PLCBayReg: 40001},
		{ID: "S2", Name: "spill bay 2", Kind: "tainter", WidthM: 12, MaxOpenM: 8, MinOpenM: 0, TravelMPerS: 0.04, PositionM: 0.5, Enabled: true, PLCBayReg: 40002},
		{ID: "S3", Name: "spill bay 3", Kind: "tainter", WidthM: 12, MaxOpenM: 8, MinOpenM: 0, TravelMPerS: 0.04, PositionM: 1.0, Enabled: true, PLCBayReg: 40003},
		{ID: "S4", Name: "spill bay 4", Kind: "tainter", WidthM: 12, MaxOpenM: 8, MinOpenM: 0, TravelMPerS: 0.04, PositionM: 0.0, Enabled: true, PLCBayReg: 40004},
	}
}

type Catalog struct {
	mu   sync.Mutex
	bays map[string]Bay
}

func NewCatalog(bays []Bay) *Catalog {
	c := &Catalog{bays: make(map[string]Bay, len(bays))}
	if len(bays) == 0 {
		bays = DefaultCatalog()
	}
	for _, b := range bays {
		id := strings.ToUpper(strings.TrimSpace(b.ID))
		if id == "" {
			continue
		}
		b.ID = id
		c.bays[id] = b
	}
	return c
}

func (c *Catalog) Get(id string) (Bay, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.bays[strings.ToUpper(strings.TrimSpace(id))]
	return b, ok
}

func (c *Catalog) List() []Bay {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Bay, 0, len(c.bays))
	for _, b := range c.bays {
		out = append(out, b)
	}
	return out
}

func (c *Catalog) Restore(bays []Bay) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bays = make(map[string]Bay, len(bays))
	for _, b := range bays {
		c.bays[b.ID] = b
	}
}

// CanTravel checks that the requested opening is inside the bay envelope
// and that the travel time is not absurd for a single command.
func (c *Catalog) CanTravel(bayID string, openingM float64) error {
	b, ok := c.Get(bayID)
	if !ok {
		return fmt.Errorf("unknown spillway bay %q", bayID)
	}
	if !b.Enabled {
		return fmt.Errorf("bay %s is locked out of service", b.ID)
	}
	if openingM < b.MinOpenM || openingM > b.MaxOpenM {
		return fmt.Errorf("bay %s opening %.2f m outside [%.2f, %.2f]", b.ID, openingM, b.MinOpenM, b.MaxOpenM)
	}
	if b.TravelMPerS <= 0 {
		return fmt.Errorf("bay %s has no travel rate configured", b.ID)
	}
	delta := openingM - b.PositionM
	if delta < 0 {
		delta = -delta
	}
	sec := delta / b.TravelMPerS
	if sec > 45*60 {
		return fmt.Errorf("bay %s travel %.0fs exceeds 45 minute command window", b.ID, sec)
	}
	return nil
}

func (c *Catalog) ApplyPosition(bayID string, openingM float64) error {
	if err := c.CanTravel(bayID, openingM); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := strings.ToUpper(strings.TrimSpace(bayID))
	b := c.bays[id]
	b.PositionM = openingM
	c.bays[id] = b
	return nil
}

// DischargeCMS is a weir approximation used to sanity-check a raise.
func DischargeCMS(b Bay, headM, openingM float64) float64 {
	if headM <= 0 || openingM <= 0 || !b.Enabled {
		return 0
	}
	cd := 0.62
	g := 9.81
	effective := openingM
	if effective > b.MaxOpenM {
		effective = b.MaxOpenM
	}
	return cd * b.WidthM * effective * mathSqrt(2*g*headM)
}

func mathSqrt(v float64) float64 {
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 12; i++ {
		x = 0.5 * (x + v/x)
	}
	return x
}
