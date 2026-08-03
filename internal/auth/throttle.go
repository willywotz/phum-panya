package auth

import (
	"sync"
	"time"

	"phum-panya/internal/clock"
)

// Throttle tracks recent failures per key within a sliding time window,
// used to rate-limit repeated failed login attempts.
type Throttle struct {
	clk    clock.Clock
	max    int
	window time.Duration

	mu       sync.Mutex
	failures map[string][]time.Time
}

// NewThrottle returns a Throttle that blocks a key once it has max or more
// failures within window.
func NewThrottle(clk clock.Clock, max int, window time.Duration) *Throttle {
	return &Throttle{
		clk:      clk,
		max:      max,
		window:   window,
		failures: make(map[string][]time.Time),
	}
}

// Allowed reports whether key has fewer than max failures within window.
func (t *Throttle) Allowed(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.prune(key)) < t.max
}

// Fail records a failure for key at the current time.
func (t *Throttle) Fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[key] = append(t.prune(key), t.clk.Now())
}

// Reset clears all recorded failures for key.
func (t *Throttle) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, key)
}

// prune removes timestamps for key that fall outside window and returns
// the remaining ones. Callers must hold t.mu.
func (t *Throttle) prune(key string) []time.Time {
	cutoff := t.clk.Now().Add(-t.window)
	kept := t.failures[key][:0]
	for _, ts := range t.failures[key] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	t.failures[key] = kept
	return kept
}
