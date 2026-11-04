package auth

import (
	"sync"
	"time"
)

// Limiter tracks consecutive failures per key and applies exponential lockout.
type Limiter struct {
	Threshold int
	BaseLock  time.Duration
	MaxLock   time.Duration

	mu sync.Mutex
	m  map[string]*entry
}

type entry struct {
	fails       int
	lockedUntil time.Time
	last        time.Time
}

func NewLimiter(threshold int, base, max time.Duration) *Limiter {
	return &Limiter{Threshold: threshold, BaseLock: base, MaxLock: max, m: map[string]*entry{}}
}

// Allow reports whether key may attempt now; otherwise how long to wait.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.m[key]; e != nil {
		if d := time.Until(e.lockedUntil); d > 0 {
			return false, d
		}
	}
	return true, 0
}

func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.m[key]
	if e == nil {
		e = &entry{}
		l.m[key] = e
	}
	e.fails++
	e.last = time.Now()
	if e.fails >= l.Threshold {
		shift := e.fails - l.Threshold
		if shift > 16 {
			shift = 16
		}
		d := l.BaseLock << shift
		if d > l.MaxLock || d <= 0 {
			d = l.MaxLock
		}
		e.lockedUntil = time.Now().Add(d)
	}
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.m, key)
	l.mu.Unlock()
}

// Sweep drops stale entries so the map can't grow without bound.
func (l *Limiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, e := range l.m {
		if time.Since(e.last) > l.MaxLock+time.Hour && time.Now().After(e.lockedUntil) {
			delete(l.m, k)
		}
	}
}
