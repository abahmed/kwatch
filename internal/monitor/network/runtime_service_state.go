package network

import (
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

func (r *Runtime) sustainServiceFinding(
	serviceKey, kind string,
	finding *model.Observation,
	duration time.Duration,
) *model.Observation {
	key := serviceKey + ":" + kind
	if finding == nil {
		r.clearServiceKey(key)
		return nil
	}
	now := r.nowTime()
	remaining := r.mark(key, now).Add(duration).Sub(now)
	if remaining > 0 {
		r.scheduleServiceRecheck(serviceKey, remaining)
		return nil
	}
	return finding
}

func (r *Runtime) scheduleServiceRecheck(
	key string, delay time.Duration,
) {
	r.mu.Lock()
	requeue := r.requeueService
	r.mu.Unlock()
	if requeue != nil {
		requeue(key, delay)
	}
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
	delete(r.firstSeen, key+":ports")
	r.mu.Unlock()
}

func (r *Runtime) clearServiceKey(key string) {
	r.mu.Lock()
	delete(r.firstSeen, key)
	r.mu.Unlock()
}
