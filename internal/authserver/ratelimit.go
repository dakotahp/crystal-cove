package authserver

import (
	"sync"
	"time"
)

const (
	registrationLimit  = 10
	registrationWindow = time.Minute
)

// rateLimit allows at most limit requests in any window, server-wide:
// behind a reverse proxy every request carries the proxy's address.
type rateLimit struct {
	now    func() time.Time
	limit  int
	window time.Duration

	mu    sync.Mutex
	times []time.Time
}

func (r *rateLimit) allow() (wait time.Duration, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	for len(r.times) > 0 && now.Sub(r.times[0]) >= r.window {
		r.times = r.times[1:]
	}
	if len(r.times) >= r.limit {
		return r.times[0].Add(r.window).Sub(now), false
	}
	r.times = append(r.times, now)
	return 0, true
}
