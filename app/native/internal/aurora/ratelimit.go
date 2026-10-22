package aurora

import (
	"strconv"
	"sync"
	"time"
)

// rate is a small sliding-window limiter for model-spending endpoints: a
// runaway client (or a stolen session) cannot burn the GLM quota.
// AURORA_SEND_PER_MIN overrides the default 20 messages per minute.
type rate struct {
	mu   sync.Mutex
	hits []time.Time
	max  int
}

func (r *rate) allow(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.max == 0 {
		r.max = 20
		if n := envInt("SEND_PER_MIN"); n > 0 {
			r.max = n
		}
	}
	keep := r.hits[:0]
	for _, t := range r.hits {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	r.hits = keep
	if len(r.hits) >= r.max {
		return false
	}
	r.hits = append(r.hits, now)
	return true
}

func envInt(name string) int { n, _ := strconv.Atoi(Env(name)); return n }
