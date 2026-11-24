// Package ratelimit bounds how often and how many at once: a token bucket per
// key and a gate for concurrent work. Both are in memory and for one process.
package ratelimit

import (
	"errors"
	"sync"
	"time"
)

// Limiter is a token bucket per key: each key may do Burst things at once and
// gets one more every Every.
type Limiter struct {
	every time.Duration
	burst float64
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	at     time.Time
}

// maxKeys is where the limiter starts to forget keys that are full again.
const maxKeys = 4096

// NewLimiter allows n events per period for each key, and n at once.
func NewLimiter(n int, per time.Duration) *Limiter {
	if n < 1 {
		n = 1
	}
	return &Limiter{every: per / time.Duration(n), burst: float64(n), now: time.Now, buckets: map[string]*bucket{}}
}

// SetClock replaces the clock, for tests.
func (l *Limiter) SetClock(now func() time.Time) { l.now = now }

// Allow takes one token of key. If there is none it reports how long until
// there is one.
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.buckets[key]
	if b == nil {
		if len(l.buckets) >= maxKeys {
			l.prune(now)
		}
		b = &bucket{tokens: l.burst, at: now}
		l.buckets[key] = b
	}
	l.refill(b, now)
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) * float64(l.every))
	return false, max(wait, time.Millisecond)
}

func (l *Limiter) refill(b *bucket, now time.Time) {
	if elapsed := now.Sub(b.at); elapsed > 0 && l.every > 0 {
		b.tokens = min(l.burst, b.tokens+float64(elapsed)/float64(l.every))
	}
	b.at = now
}

// prune forgets the keys whose bucket is full: they are as good as new.
func (l *Limiter) prune(now time.Time) {
	for k, b := range l.buckets {
		l.refill(b, now)
		if b.tokens >= l.burst {
			delete(l.buckets, k)
		}
	}
}

// Errors of a Gate.
var (
	// ErrBusy: the key already has work running.
	ErrBusy = errors.New("already running")
	// ErrFull: the gate is at its limit.
	ErrFull = errors.New("too many at once")
)

// Gate lets at most Max pieces of work run at once, and one per key.
type Gate struct {
	max int

	mu      sync.Mutex
	running map[string]struct{}
}

// NewGate returns a gate for max concurrent pieces of work.
func NewGate(max int) *Gate {
	if max < 1 {
		max = 1
	}
	return &Gate{max: max, running: map[string]struct{}{}}
}

// Acquire reserves a slot for key. The caller must call release exactly when
// the work ends; calling it again does nothing.
func (g *Gate) Acquire(key string) (release func(), err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.running[key]; ok {
		return nil, ErrBusy
	}
	if len(g.running) >= g.max {
		return nil, ErrFull
	}
	g.running[key] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			delete(g.running, key)
			g.mu.Unlock()
		})
	}, nil
}

// Running is how many slots are taken.
func (g *Gate) Running() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.running)
}
