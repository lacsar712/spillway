package hydrology

import (
	"math"

	"github.com/lacsar712/spillway/internal/reservoir"
)

// NeedSpill reports whether the pool is rising toward flood stage fast
// enough that the spillway should stay open.
func NeedSpill(r reservoir.Reading) bool {
	if r.LevelM >= r.FloodM-0.4 {
		return true
	}
	if r.SurplusCMS() > 80 && r.FreeboardM() < 2 {
		return true
	}
	h := reservoir.RiseHours(r)
	return h < 6
}

// Unsteady rejects a raise whose estimated discharge would reverse the
// tailrace slope (a crude but real operating constraint).
func Unsteady(r reservoir.Reading, dischargeCMS float64) bool {
	if dischargeCMS <= 0 {
		return false
	}
	tw := r.TailwaterM + 0.002*dischargeCMS
	return tw > r.CrestM-1.5
}

// Balance updates storage from inflow/outflow over dt seconds.
func Balance(r reservoir.Reading, dtSec float64) reservoir.Reading {
	if dtSec <= 0 {
		return r
	}
	dV := (r.InflowCMS - r.OutflowCMS) * dtSec / 1e6
	r.StorageMCM += dV
	if r.StorageMCM < 0 {
		r.StorageMCM = 0
	}
	r.LevelM += dV * 0.08
	if r.LevelM < r.MinPoolM {
		r.LevelM = r.MinPoolM
	}
	return r
}

// DesignFloodCMS is a simplified PMF-ish envelope used by operators
// to compare current inflow against the spillway rating.
func DesignFloodCMS(crestM, floodM float64) float64 {
	span := math.Max(floodM-crestM, 1)
	return 900 + 120*span
}

func RatingCMS(levelM, crestM float64, openWidthM float64) float64 {
	head := levelM - crestM
	if head <= 0 || openWidthM <= 0 {
		return 0
	}
	return 0.62 * openWidthM * math.Sqrt(2*9.81*head)
}
