package app

import (
	"sync"

	"github.com/abahmed/kwatch/internal/health"
)

// readinessCoordinator owns the required monitoring gates for one leadership
// session. Optional components report through health but do not participate in
// this gate.
type readinessCoordinator struct {
	mu              sync.Mutex
	health          *health.HealthServer
	epoch           int64
	leader          bool
	required        map[string]bool
	ready           map[string]bool
	writers         map[string]bool
	requiredWriters map[string]struct{}
}

func newReadinessCoordinator(
	server *health.HealthServer,
) *readinessCoordinator {
	return &readinessCoordinator{health: server}
}

func (r *readinessCoordinator) begin(
	epoch int64,
	deliveryRequired bool,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.epoch = epoch
	r.leader = true
	r.required = map[string]bool{
		"restore":             true,
		"controller":          true,
		"persistence-writers": true,
		"incident":            true,
	}
	if deliveryRequired {
		r.required["delivery"] = true
	}
	r.ready = make(map[string]bool, len(r.required))
	r.writers = make(map[string]bool)
	r.requiredWriters = make(map[string]struct{})
	r.setHealthLocked(false)
}

func (r *readinessCoordinator) end(epoch int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.epoch != epoch {
		return
	}
	r.leader = false
	r.setHealthLocked(false)
}

func (r *readinessCoordinator) setCurrent(name string, ready bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.leader {
		return
	}
	r.ready[name] = ready
	r.setHealthLocked(r.readyNowLocked())
}

func (r *readinessCoordinator) setForEpoch(
	epoch int64,
	name string,
	ready bool,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.epoch != epoch || !r.leader {
		return
	}
	r.ready[name] = ready
	r.setHealthLocked(r.readyNowLocked())
}

func (r *readinessCoordinator) registerRequiredWriter(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.leader {
		return
	}
	if name == "" {
		return
	}
	r.requiredWriters[name] = struct{}{}
	r.ready["persistence-writers"] = false
	r.setHealthLocked(r.readyNowLocked())
}

func (r *readinessCoordinator) writerStarted(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.leader {
		return
	}
	if name != "" {
		r.writers[name] = true
	}
	ready := len(r.requiredWriters) > 0 &&
		len(r.writers) >= len(r.requiredWriters)
	r.ready["persistence-writers"] = ready
	r.setHealthLocked(r.readyNowLocked())
}

func (r *readinessCoordinator) writerFailed(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.leader {
		return
	}
	if name != "" {
		delete(r.writers, name)
	}
	r.ready["persistence-writers"] = false
	r.setHealthLocked(false)
}

func (r *readinessCoordinator) readyNowLocked() bool {
	if !r.leader {
		return false
	}
	for name := range r.required {
		if !r.ready[name] {
			return false
		}
	}
	return true
}

func (r *readinessCoordinator) setHealthLocked(ready bool) {
	if r.health != nil {
		r.health.SetReady(ready)
	}
}
