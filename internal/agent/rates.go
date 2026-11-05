package agent

import "time"

type rateEntry struct {
	value uint64
	at    time.Time
}

// Rates turns monotonically increasing counters into per-second rates.
type Rates struct {
	prev map[string]rateEntry
}

func NewRates() *Rates { return &Rates{prev: map[string]rateEntry{}} }

// Rate records value for key and returns the per-second increase since the
// previous call. The first sighting of a key, a counter that went down (a
// reset), or no elapsed time all return ok=false and start a new baseline.
func (r *Rates) Rate(key string, value uint64, now time.Time) (perSec float64, ok bool) {
	p, seen := r.prev[key]
	r.prev[key] = rateEntry{value, now}
	if !seen || value < p.value {
		return 0, false
	}
	secs := now.Sub(p.at).Seconds()
	if secs <= 0 {
		return 0, false
	}
	return float64(value-p.value) / secs, true
}
