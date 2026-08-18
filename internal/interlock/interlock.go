package interlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/lacsar712/spillway/internal/event"
	"github.com/lacsar712/spillway/internal/gate"
	"github.com/lacsar712/spillway/internal/hydrology"
	"github.com/lacsar712/spillway/internal/reservoir"
)

var (
	ErrLowPool     = errors.New("reservoir below spillway crest")
	ErrFloodClose  = errors.New("cannot close gates during flood")
	ErrUnknownMove = errors.New("unsupported gate command type")
	ErrSubmerged   = errors.New("tailwater submergence blocks the move")
	ErrBay         = errors.New("bay envelope rejected the move")
)

// Move is the operator intent extracted from a signed command payload.
type Move struct {
	Bay      string  `json:"bay"`
	OpeningM float64 `json:"opening_m"`
	Reason   string  `json:"reason"`
}

type Guard struct {
	Pool *reservoir.Pool
	Bays *gate.Catalog
}

func New(pool *reservoir.Pool, bays *gate.Catalog) *Guard {
	if pool == nil {
		pool = reservoir.New(reservoir.Default())
	}
	if bays == nil {
		bays = gate.NewCatalog(nil)
	}
	return &Guard{Pool: pool, Bays: bays}
}

func Permissive() *Guard {
	return New(reservoir.New(reservoir.Default()), gate.NewCatalog(nil))
}

func ParseMove(env event.Envelope) (Move, error) {
	var raw struct {
		Bay      string  `json:"bay"`
		OpeningM float64 `json:"opening_m"`
		Reason   string  `json:"reason"`
	}
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &raw); err != nil {
			return Move{}, fmt.Errorf("payload: %w", err)
		}
	}
	m := Move{Bay: strings.ToUpper(strings.TrimSpace(raw.Bay)), OpeningM: raw.OpeningM, Reason: strings.TrimSpace(raw.Reason)}
	if m.Bay == "" {
		m.Bay = "S1"
	}
	if m.OpeningM == 0 {
		switch env.Type {
		case "gate.raise":
			m.OpeningM = 1.5
		case "gate.lower":
			m.OpeningM = 0
		case "gate.hold", "gate.stop":
			if b, ok := gate.NewCatalog(nil).Get(m.Bay); ok {
				m.OpeningM = b.PositionM
			}
		}
	}
	return m, nil
}

// Allow is invoked on the command accept path after the body parses and
// before nonce/idempotency commit. A rejected move never reaches a PLC.
func (g *Guard) Allow(env event.Envelope) error {
	if g == nil || g.Pool == nil {
		return fmt.Errorf("interlock is not configured")
	}
	move, err := ParseMove(env)
	if err != nil {
		return err
	}
	r := g.Pool.Snapshot()
	switch env.Type {
	case "gate.raise":
		if r.LevelM < r.CrestM {
			return fmt.Errorf("%w: level=%.2f crest=%.2f", ErrLowPool, r.LevelM, r.CrestM)
		}
		if r.TailwaterM > r.CrestM-2 {
			return fmt.Errorf("%w: tailwater %.2f m", ErrSubmerged, r.TailwaterM)
		}
		if err := g.Bays.CanTravel(move.Bay, move.OpeningM); err != nil {
			return fmt.Errorf("%w: %v", ErrBay, err)
		}
		b, _ := g.Bays.Get(move.Bay)
		q := gate.DischargeCMS(b, r.HeadM(), move.OpeningM)
		if hydrology.Unsteady(r, q) {
			return fmt.Errorf("%w: requested discharge %.0f m3/s would drown tailrace", ErrSubmerged, q)
		}
		return nil
	case "gate.lower":
		if r.LevelM >= r.FloodM {
			return fmt.Errorf("%w: level=%.2f flood=%.2f", ErrFloodClose, r.LevelM, r.FloodM)
		}
		if hydrology.NeedSpill(r) && move.OpeningM < 0.3 {
			return fmt.Errorf("%w: inflow still requires spill", ErrFloodClose)
		}
		if err := g.Bays.CanTravel(move.Bay, move.OpeningM); err != nil {
			return fmt.Errorf("%w: %v", ErrBay, err)
		}
		return nil
	case "gate.hold", "gate.stop":
		return nil
	default:
		if strings.HasPrefix(env.Type, "gate.") {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrUnknownMove, env.Type)
	}
}
