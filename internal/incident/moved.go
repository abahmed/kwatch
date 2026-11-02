package incident

// maxMovedFrom bounds Moved.From: a story that keeps absorbing others
// lists only the latest ones.
const maxMovedFrom = 10

// Moved links an incident with the ones it was moved into or that were
// moved into it, for the dashboard. Only moves people were told about are
// kept: the closing "Moved" message was decided. A quiet supersede (the
// same story under another root, closed without a message) is not one.
// Persisted, so the links survive a restart.
type Moved struct {
	// Into is the ID of the incident this one was moved into.
	Into string
	// From lists the IDs of the earlier incidents moved into this one,
	// oldest first, at most maxMovedFrom.
	From []string
}

func (v Moved) clone() Moved {
	v.From = append([]string(nil), v.From...)
	return v
}

// recordMove links p, whose "Moved" resolve is being decided, with the
// incident that took its failures over.
func (m *Manager) recordMove(p *Incident) {
	if p.SupersededBy == "" {
		return
	}
	p.Moved.Into = p.SupersededBy
	target := m.incidents[p.SupersededBy]
	if target == nil {
		return
	}
	target.Moved.From = appendMovedFrom(target.Moved.From, p.ID)
}

// appendMovedFrom adds id once, keeping the latest maxMovedFrom.
func appendMovedFrom(from []string, id string) []string {
	for _, have := range from {
		if have == id {
			return from
		}
	}
	from = append(from, id)
	if n := len(from); n > maxMovedFrom {
		from = append([]string(nil), from[n-maxMovedFrom:]...)
	}
	return from
}
