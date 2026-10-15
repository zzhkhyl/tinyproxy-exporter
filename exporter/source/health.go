package source

import (
	"sync"
	"time"
)

// HealthSnapshot is a point in time view of a data source's reachability.
type HealthSnapshot struct {
	// Up is true when the most recent attempt succeeded.
	Up bool
	// LastSuccess is when an attempt last succeeded (zero if never).
	LastSuccess time.Time
	// LastError is the message of the most recent failure ("" when up).
	LastError string
	// Duration is how long the most recent attempt took.
	Duration time.Duration
	// Failures counts failed attempts.
	Failures uint64
	// Attempts counts all attempts.
	Attempts uint64
}

// Health records the success/failure history of a data source.
type Health struct {
	mu       sync.Mutex
	up       bool
	lastOK   time.Time
	lastErr  error
	duration time.Duration
	failures uint64
	attempts uint64
}

// MarkSuccess records a successful attempt.
func (h *Health) MarkSuccess(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.up = true
	h.lastOK = time.Now()
	h.lastErr = nil
	h.duration = d
	h.attempts++
}

// MarkFailure records a failed attempt.
func (h *Health) MarkFailure(err error, d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.up = false
	h.lastErr = err
	h.duration = d
	h.failures++
	h.attempts++
}

// Snapshot returns a consistent copy of the current state.
func (h *Health) Snapshot() HealthSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := HealthSnapshot{
		Up:          h.up,
		LastSuccess: h.lastOK,
		Duration:    h.duration,
		Failures:    h.failures,
		Attempts:    h.attempts,
	}
	if h.lastErr != nil {
		s.LastError = h.lastErr.Error()
	}
	return s
}
