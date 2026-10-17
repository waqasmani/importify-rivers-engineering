// Package ratelimit provides keyed token-bucket rate limiting with idle-key eviction.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type entry struct {
	lim  *rate.Limiter
	seen time.Time
}

// Limiter limits events per key (for example per client IP).
type Limiter struct {
	mu      sync.Mutex
	keys    map[string]*entry
	every   rate.Limit
	burst   int
	maxKeys int
}

// New allows `perMinute` events per minute per key with the given burst.
func New(perMinute float64, burst int) *Limiter {
	return &Limiter{keys: map[string]*entry{}, every: rate.Limit(perMinute / 60), burst: burst, maxKeys: 100000}
}

// Allow reports whether an event for key may happen now.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e, ok := l.keys[key]
	if !ok {
		if len(l.keys) >= l.maxKeys {
			l.evict(now, 0)
		}
		e = &entry{lim: rate.NewLimiter(l.every, l.burst)}
		l.keys[key] = e
	}
	e.seen = now
	return e.lim.Allow()
}

// Sweep removes keys idle for longer than idle. Call periodically.
func (l *Limiter) Sweep(idle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evict(time.Now(), idle)
}

func (l *Limiter) evict(now time.Time, idle time.Duration) {
	if idle == 0 {
		idle = time.Minute
	}
	for k, e := range l.keys {
		if now.Sub(e.seen) > idle {
			delete(l.keys, k)
		}
	}
}
