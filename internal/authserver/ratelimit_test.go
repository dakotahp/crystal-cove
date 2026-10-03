package authserver

import (
	"testing"
	"time"
)

func TestRateLimitAllowsLimitPerWindow(t *testing.T) {
	clock := newTestClock()
	r := &rateLimit{now: clock.now, limit: 3, window: time.Minute}
	for i := range 3 {
		if _, ok := r.allow(); !ok {
			t.Fatalf("request %d refused below the limit", i+1)
		}
		clock.advance(10 * time.Second)
	}
	wait, ok := r.allow()
	if ok || wait != 30*time.Second {
		t.Fatalf("over the limit: wait = %v, ok = %v; want 30s, false", wait, ok)
	}
	clock.advance(wait)
	if _, ok := r.allow(); !ok {
		t.Error("refused after the oldest request left the window")
	}
	if _, ok := r.allow(); ok {
		t.Error("allowed a second request while the window is still full")
	}
}

func TestRateLimitRefusalUsesNoSlot(t *testing.T) {
	clock := newTestClock()
	r := &rateLimit{now: clock.now, limit: 1, window: time.Minute}
	r.allow()
	for range 5 {
		r.allow()
	}
	clock.advance(time.Minute)
	if _, ok := r.allow(); !ok {
		t.Error("refused requests extended the window")
	}
}
