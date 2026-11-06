package telemetrycollector

import (
	"sync"
	"time"
)

// RateLimiter is a per-key token bucket held entirely in memory. Keys are
// remote addresses; the limiter never persists a key to disk and never logs
// one. Callers are responsible for choosing what key to pass (see
// clientKey in handlers.go), and for periodically calling Sweep to bound
// memory growth from one-off callers.
type RateLimiter struct {
	mu    sync.Mutex
	rate  float64 // tokens added per second
	burst float64 // bucket capacity, equal to the configured per-minute limit
	now   func() time.Time

	buckets map[string]*bucket
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// NewRateLimiter builds a limiter that allows perMinute requests per key,
// refilled continuously (perMinute tokens every 60 seconds) up to a burst
// capacity of perMinute. A non-positive perMinute disables limiting: Allow
// always returns true.
func NewRateLimiter(perMinute int) *RateLimiter {
	return &RateLimiter{
		rate:    float64(perMinute) / 60.0,
		burst:   float64(perMinute),
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether a request for key may proceed, consuming one token
// if so.
func (l *RateLimiter) Allow(key string) bool {
	if l.burst <= 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, lastRefill: now}
		l.buckets[key] = b
	}
	b.lastSeen = now

	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastRefill = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Sweep evicts buckets that have not been used in longer than maxIdle,
// bounding memory when many distinct remote addresses come and go.
func (l *RateLimiter) Sweep(maxIdle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for key, b := range l.buckets {
		if now.Sub(b.lastSeen) > maxIdle {
			delete(l.buckets, key)
		}
	}
}
