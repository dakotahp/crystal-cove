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

func (l *limiter) attempt(check func() bool) (wait time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if wait := l.lockedUntil.Sub(l.now()); wait > 0 {
		return wait, false
	}
	if check() {
		l.failures = 0
		l.lockedUntil = time.Time{}
		return 0, true
	}
	l.failures++
	if l.failures >= freeAttempts {
		shift := min(l.failures-freeAttempts, 6)
		l.lockedUntil = l.now().Add(min(firstLock<<shift, maxLock))
	}
	return 0, false
}
