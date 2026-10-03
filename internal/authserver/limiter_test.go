package authserver

import (
	"testing"
	"time"
)

func TestLimiterLocksAfterFiveFailuresAndDoubles(t *testing.T) {
	clock := newTestClock()
	l := &limiter{now: clock.now}
	for range 4 {
		wrong(l)
	}
	if w := remaining(l); w != 0 {
		t.Fatalf("locked after 4 failures: %v", w)
	}

	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour}
	for i, d := range want {
		wrong(l)
		if w := remaining(l); w != d {
			t.Fatalf("failure %d: wait = %v, want %v", i+5, w, d)
		}
		clock.advance(d)
		if w := remaining(l); w != 0 {
			t.Fatalf("still locked after the lock passed: %v", w)
		}
	}
}

func TestLimiterSuccessResets(t *testing.T) {
	clock := newTestClock()
	l := &limiter{now: clock.now}
	for range 5 {
		wrong(l)
	}
	clock.advance(time.Hour)
	right(l)
	if w := remaining(l); w != 0 {
		t.Errorf("wait after success = %v", w)
	}
	wrong(l)
	if w := remaining(l); w != 0 {
		t.Errorf("one failure after a reset locked the form: %v", w)
	}
}

func wrong(l *limiter) { l.attempt(func() bool { return false }) }

func right(l *limiter) (time.Duration, bool) { return l.attempt(func() bool { return true }) }

func remaining(l *limiter) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return max(l.lockedUntil.Sub(l.now()), 0)
}

func TestLimiterDoesNotCheckWhileLocked(t *testing.T) {
	l := &limiter{now: newTestClock().now}
	for range 5 {
		wrong(l)
	}
	checked := false
	wait, ok := l.attempt(func() bool { checked = true; return true })
	if checked || ok || wait != time.Minute {
		t.Errorf("checked = %v, ok = %v, wait = %v", checked, ok, wait)
	}
}
