package incident

import "github.com/abahmed/kwatch/internal/detection"

// deletionFinishing reports a stuck deletion, a finding of the announced
// incident previous, that would move back to its own root only because
// the cause it was stuck on recovered. The deletion is finishing: the
// root people were told stays until it resolves, instead of a second
// "cause revised" naming the object they already know.
func (m *Manager) deletionFinishing(
	previous string, s detection.Finding, where placement,
) bool {
	p := m.incidents[previous]
	if p == nil || !wasAnnounced(p) || where.cause != nil ||
		where.root != s.Entity || p.Root == s.Entity {
		return false
	}
	return s.Mode.Within(detection.ModeStuckDeleting) ||
		s.Mode.Within(detection.ModeTerminating)
}

// draining reports an incident whose root recovered while the
// deletions it blocked are still finishing: every remaining member is a
// stuck deletion and none is the root's own. The recovery is no news of
// its own; the resolve closes the thread.
func draining(p *Incident) bool {
	if len(p.Members) == 0 {
		return false
	}
	for key, s := range p.Members {
		if key.Entity == p.Root || !(s.Mode.Within(
			detection.ModeStuckDeleting) ||
			s.Mode.Within(detection.ModeTerminating)) {
			return false
		}
	}
	return true
}
