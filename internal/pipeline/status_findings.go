package pipeline

import (
	"sync"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// publishedFindings is a copy of the active findings that other
// goroutines may read: the engine's own map belongs to the decision
// loop. The loop is the only writer; /status and the digest hook read.
type publishedFindings struct {
	mu       sync.RWMutex
	byEntity map[inventory.EntityID][]detection.Finding
}

// set publishes the active findings of id; none removes the entry.
func (p *publishedFindings) set(
	id inventory.EntityID, active []detection.Finding,
) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(active) == 0 {
		delete(p.byEntity, id)
		return
	}
	if p.byEntity == nil {
		p.byEntity = map[inventory.EntityID][]detection.Finding{}
	}
	p.byEntity[id] = active
}

// all returns every published finding, a fresh slice.
func (p *publishedFindings) all() []detection.Finding {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []detection.Finding
	for _, found := range p.byEntity {
		out = append(out, found...)
	}
	return out
}
