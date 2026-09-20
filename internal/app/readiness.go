package app

import (
	"sync"

	"github.com/abahmed/kwatch/internal/health"
)

// readinessCoordinator owns the required monitoring gates for one leadership
// session. Optional components report through health but do not participate in
// this gate.
type readinessCoordinator struct {
	mu       sync.Mutex
	health   *health.HealthServer
	epoch    int64
	leader   bool
	required map[string]bool
	ready    map[string]bool
	writers  int
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
	r.writers = 0
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

func (r *readinessCoordinator) writerStarted() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.leader {
		return
	}
	r.writers++
	if r.writers >= 3 {
		r.ready["persistence-writers"] = true
	}
	r.setHealthLocked(r.readyNowLocked())
}

func (r *readinessCoordinator) writerFailed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.leader {
		return
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
