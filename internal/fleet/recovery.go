package fleet

import "time"

// Recovery caps repeated crashes without changing the server's watchdog.
// The caller serializes this policy with fleet actions.
type Recovery struct{ attempts []time.Time }

func (r *Recovery) Allow(now time.Time) bool {
	kept := r.attempts[:0]
	for _, at := range r.attempts {
		if now.Sub(at) < 15*time.Minute {
			kept = append(kept, at)
		}
	}
	r.attempts = kept
	if len(kept) >= 3 || (len(kept) > 0 && now.Sub(kept[len(kept)-1]) < time.Minute) {
		return false
	}
	r.attempts = append(r.attempts, now)
	return true
}
func (r *Recovery) Reset() { r.attempts = nil }
