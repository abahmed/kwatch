package incident

import (
	"slices"
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
// or crashed within the last hold. The pods come from the root, or from
// the objects the timeline names when the root, such as a missing
// Service, has no pods of its own.
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
// with a non-zero exit less than hold ago. A clean exit is a normal
// end, such as an init container's, not a crash.
func crashed(e inventory.Entity, now time.Time, hold time.Duration) bool {
	if a, ok := e.Attribute(kube.AttrStateReason); ok &&
		a.Value.AsText() == reasons.CrashLoopBackOff {
		return true
	}
	code, ok := e.Attribute(kube.AttrLastExitCode)
	if !ok {
		return false
	}
	if n, isNumber := code.Value.AsNumber(); !isNumber || n == 0 {
		return false
	}
	a, ok := e.Attribute(kube.AttrLastFinished)
	if !ok {
		return false
	}
	finished := a.Value.AsTime()
	return !finished.IsZero() && now.Sub(finished) < hold
}

// podsOf lists the pods that tell whether p still fails: those of the
// workloads behind its root, or, for a root without pods of its own,
// those of the workloads its timeline names. For a node root only the
// named workloads' pods on that node count: a crash loop elsewhere is
// not the node's.
func (m *Manager) podsOf(p *Incident) []inventory.EntityID {
	var pods []inventory.EntityID
	for _, owner := range workloadsOf(m.model, workloadFor(m.model,
		p.Root)) {
		pods = append(pods, rootcause.OwnedPods(m.model, owner)...)
	}
	if len(pods) > 0 {
		return pods
	}
	for _, owner := range m.namedWorkloads(p) {
		for _, pod := range rootcause.OwnedPods(m.model, owner) {
			if p.Root.Kind != kube.KindNode || m.runsOn(pod, p.Root) {
				pods = append(pods, pod)
			}
		}
	}
	return pods
}

// namedWorkloads are the workloads of the objects p's timeline names,
// each once.
func (m *Manager) namedWorkloads(p *Incident) []inventory.EntityID {
	var out []inventory.EntityID
	for _, ev := range p.Timeline {
		if ev.Entity == nil {
			continue
		}
		for _, w := range workloadsOf(m.model, *ev.Entity) {
			if !slices.Contains(out, w) {
				out = append(out, w)
			}
		}
	}
	return out
}

// runsOn reports a pod scheduled on node.
func (m *Manager) runsOn(pod, node inventory.EntityID) bool {
	return slices.Contains(m.model.Related(pod, inventory.RunsOn,
		inventory.Outgoing), node)
}
