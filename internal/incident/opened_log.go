package incident

import (
	"time"

	"k8s.io/klog/v2"
)

// freshIncident is an incident opened in the current Apply and the
// reason of the finding that opened it.
type freshIncident struct {
	id     string
	reason string
}

// logOpened writes one default-level line per incident opened in this
// Apply, once its tier is known, so the settle window before the first
// message is not silent. An incident merged away in the same Apply, or
// left without members, is not logged.
func (m *Manager) logOpened() {
	for _, f := range m.fresh {
		p := m.incidents[f.id]
		if p == nil || p.State != Settling || len(p.Members) == 0 {
			continue
		}
		klog.InfoS("incident opened",
			"component", "incident", "operation", "open",
			"incident", p.ID, "namespace", p.Root.Namespace,
			"kind", string(p.Root.Kind), "name", p.Root.Name,
			"reason", f.reason, "tier", p.Tier.String(),
			"sendsAt", m.sendsAt(p))
	}
	m.fresh = m.fresh[:0]
}

// sendsAt says when p's first message is due, or "never" for a silent
// incident. A later finding can still change the tier and the time.
func (m *Manager) sendsAt(p *Incident) string {
	if p.Tier == Silent {
		return "never"
	}
	return p.Opened.Add(m.settleFor(p)).UTC().Format(time.RFC3339)
}
