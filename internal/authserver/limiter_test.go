package authserver

import (
	"testing"
	"time"
)

func TestLimiterLocksAfterFiveFailuresAndDoubles(t *testing.T) {
	clock := newTestClock()
	l := &limiter{now: clock.now}
	for range 4 {
		l.fail()
	}
	if w := l.wait(); w != 0 {
		t.Fatalf("locked after 4 failures: %v", w)
	}

	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour}
	for i, d := range want {
		l.fail()
		if w := l.wait(); w != d {
			t.Fatalf("failure %d: wait = %v, want %v", i+5, w, d)
		}
		clock.advance(d)
		if w := l.wait(); w != 0 {
			t.Fatalf("still locked after the lock passed: %v", w)
		}
	}
}

func TestLimiterSuccessResets(t *testing.T) {
	clock := newTestClock()
	l := &limiter{now: clock.now}
	for range 5 {
		l.fail()
	}
	l.succeed()
	if w := l.wait(); w != 0 {
		t.Errorf("wait after success = %v", w)
	}
	l.fail()
	if w := l.wait(); w != 0 {
		t.Errorf("one failure after a reset locked the form: %v", w)
	}
}
