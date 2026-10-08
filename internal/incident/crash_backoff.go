package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A crashing container is silent between two crashes: it waits in
// back-off, up to five minutes, and the hold before a resolve is
// shorter. Without these checks the quiet would read as health and the
// incident would resolve in the middle of the crash loop.

// crashing reports a container of p's workloads that waits in back-off
// or ended a run within the last hold. The pods come from the root and
// from the objects the timeline names, as a root such as a missing
// Service has no pods of its own.
func (m *Manager) crashing(p *Incident, now time.Time) bool {
	if m.model == nil {
		return false
	}
	hold := m.holdFor(p, now)
	for _, pod := range m.podsOf(p) {
		for _, c := range m.model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			if c.Kind != kube.KindContainer {
				continue
			}
			if e, ok := m.model.Entity(c); ok && crashed(e, now, hold) {
				return true
			}
		}
	}
	return false
}

// crashed reports a container in back-off, or one whose last run ended
// less than hold ago.
func crashed(e inventory.Entity, now time.Time, hold time.Duration) bool {
	if a, ok := e.Attribute(kube.AttrStateReason); ok &&
		a.Value.AsText() == reasons.CrashLoopBackOff {
		return true
	}
	a, ok := e.Attribute(kube.AttrLastFinished)
	if !ok {
		return false
	}
	finished := a.Value.AsTime()
	return !finished.IsZero() && now.Sub(finished) < hold
}

// podsOf lists the pods of the workloads behind p's root and of the
// objects its timeline names.
func (m *Manager) podsOf(p *Incident) []inventory.EntityID {
	owners := workloadsOf(m.model, workloadFor(m.model, p.Root))
	for _, ev := range p.Timeline {
		if ev.Entity != nil {
			owners = append(owners, workloadsOf(m.model, *ev.Entity)...)
		}
	}
	var pods []inventory.EntityID
	for _, owner := range owners {
		pods = append(pods, rootcause.OwnedPods(m.model, owner)...)
	}
	return pods
}
