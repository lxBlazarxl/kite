package agent

import (
	"sync"
	"time"
)

// cached refreshes a value at most once per period, keeping the last good
// value across failures. Used for WMI, which is too expensive to query on
// every 5 s sample while gaming. With async set, the refresh runs in the
// background so a sample never waits for WMI.
type cached[T any] struct {
	every time.Duration
	fetch func() (T, error)
	async bool

	mu       sync.Mutex
	tried    time.Time
	inFlight bool
	value    T
	hasValue bool
}

func newCached[T any](every time.Duration, fetch func() (T, error)) *cached[T] {
	return &cached[T]{every: every, fetch: fetch}
}

// newAsyncCached is like newCached but never blocks the caller: a stale
// value triggers a background refresh and the previous value is returned.
func newAsyncCached[T any](every time.Duration, fetch func() (T, error)) *cached[T] {
	return &cached[T]{every: every, fetch: fetch, async: true}
}

func (c *cached[T]) get(now time.Time) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.inFlight && (c.tried.IsZero() || now.Sub(c.tried) >= c.every) {
		c.tried = now
		if c.async {
			c.inFlight = true
			go c.refresh()
		} else {
			if v, err := c.fetch(); err == nil {
				c.value, c.hasValue = v, true
			}
		}
	}
	return c.value, c.hasValue
}

func (c *cached[T]) refresh() {
	v, err := c.fetch()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inFlight = false
	if err == nil {
		c.value, c.hasValue = v, true
	}
}
