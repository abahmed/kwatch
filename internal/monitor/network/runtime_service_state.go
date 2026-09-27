package network

import (
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

func (r *Runtime) sustainServiceFinding(
	key string,
	finding *model.Observation,
	duration time.Duration,
) *model.Observation {
	if finding == nil {
		r.clearServiceKey(key)
		return nil
	}
	now := r.nowTime()
	if now.Sub(r.mark(key, now)) < duration {
		return nil
	}
	return finding
}

func (r *Runtime) mark(key string, now time.Time) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if first, ok := r.firstSeen[key]; ok {
		return first
	}
	r.firstSeen[key] = now
	return now
}

func (r *Runtime) clearService(namespace, name string) {
	r.mu.Lock()
	key := namespace + "/" + name
	delete(r.firstSeen, key+":outage")
	delete(r.firstSeen, key+":degraded")
	r.mu.Unlock()
}

func (r *Runtime) clearServiceKey(key string) {
	r.mu.Lock()
	delete(r.firstSeen, key)
	r.mu.Unlock()
}
