package authserver

import (
	"sync"
	"time"
)

const (
	freeAttempts = 5
	firstLock    = time.Minute
	maxLock      = time.Hour
)

// limiter locks the sign-in form after repeated wrong passwords. One
// counter covers the whole server, because there is one owner.
type limiter struct {
	now func() time.Time

	mu          sync.Mutex
	failures    int
	lockedUntil time.Time
}

func (l *limiter) wait() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return max(l.lockedUntil.Sub(l.now()), 0)
}

func (l *limiter) fail() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures++
	if l.failures < freeAttempts {
		return
	}
	shift := min(l.failures-freeAttempts, 6)
	l.lockedUntil = l.now().Add(min(firstLock<<shift, maxLock))
}

func (l *limiter) succeed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures = 0
	l.lockedUntil = time.Time{}
}
