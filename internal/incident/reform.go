package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// reform gives the failures that outlived a resolved incident an incident
// of their own, so a closed incident never leaves a failing object
// unowned. They are placed under their own workload and settle and
// announce like any new failure.
func (m *Manager) reform(p *Incident, now time.Time) {
	if len(p.Members) == 0 {
		return
	}
	left := p.Members
	p.Members = map[detection.Key]detection.Finding{}
	for key, f := range left {
		delete(m.byMember, key)
		if f.Advisory {
			continue
		}
		root := f.Entity
		if m.model != nil {
			root = defaultRoot(m.model, f.Entity)
		}
		m.attach(now, f, placement{root: root})
	}
	if m.model != nil {
		// Outside Apply nothing else computes the new incidents' tier.
		m.refresh(m.model, now)
	}
}
