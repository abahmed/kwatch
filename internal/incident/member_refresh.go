package incident

import "github.com/abahmed/kwatch/internal/detection"

// refreshMember replaces the copy of a finding an incident holds with
// the latest one, when only its wording moved (detection.Refreshed). It
// changes nothing else: no placement, no timeline event, no regrouping.
// A finding no incident holds is ignored; it is placed when it is
// raised or changes.
func (m *Manager) refreshMember(f detection.Finding) {
	key := f.Key()
	if p := m.incidents[m.byMember[key]]; p != nil {
		if _, held := p.Members[key]; held {
			p.Members[key] = f
		}
	}
}
