package analog

import "fmt"

// Scale maps an engineering value onto a PLC integer register.
func Scale(eng, minEng, maxEng float64, minRaw, maxRaw int) (int, error) {
	if maxEng == minEng {
		return 0, fmt.Errorf("zero engineering span")
	}
	if maxRaw == minRaw {
		return 0, fmt.Errorf("zero raw span")
	}
	frac := (eng - minEng) / (maxEng - minEng)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	raw := float64(minRaw) + frac*float64(maxRaw-minRaw)
	return int(raw + 0.5), nil
}

func Unscale(raw, minRaw, maxRaw int, minEng, maxEng float64) (float64, error) {
	if maxRaw == minRaw {
		return 0, fmt.Errorf("zero raw span")
	}
	frac := float64(raw-minRaw) / float64(maxRaw-minRaw)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return minEng + frac*(maxEng-minEng), nil
}

func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func Deadband(prev, next, band float64) float64 {
	d := next - prev
	if d < 0 {
		d = -d
	}
	if d < band {
		return prev
	}
	return next
}
