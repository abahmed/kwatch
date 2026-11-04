package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// flipDamping is how long a finding that left a root stays where it
// went when the reasoning blames that root again. Between two looks a
// crash loop or a node blip can make the solver swing between two
// candidates for the very same failures; each swing would be a cause
// revision, a timeline note and a fingerprint change.
const flipDamping = 2 * time.Minute

// flip is the root a finding last left and when.
type flip struct {
	from inventory.EntityID
	at   time.Time
}

// swingsBack reports a finding whose cause returns to the root it left
// less than flipDamping ago: it stays where it is. A swing to a third
// root, or a return after the damping, is a real revision, and so is a
// return to the finding's own object or to a root that now fails by
// itself: those are evidence, not a swing of the reasoning.
func (m *Manager) swingsBack(
	now time.Time, key detection.Key, root inventory.EntityID,
) bool {
	f, ok := m.flips[key]
	if !ok {
		return false
	}
	if now.Sub(f.at) >= flipDamping {
		delete(m.flips, key)
		return false
	}
	return f.from == root && root != key.Entity &&
		!m.failsByItself(root)
}

// failsByItself reports a root whose own object has a live failure.
func (m *Manager) failsByItself(root inventory.EntityID) bool {
	p := m.lookup(root)
	if p == nil || p.State == Resolved {
		return false
	}
	for key, s := range p.Members {
		if key.Entity == root && !s.Advisory && !s.Symptom {
			return true
		}
	}
	return false
}

// failingFinding reports a failure among an object's active findings.
func failingFinding(findings []detection.Finding) bool {
	for _, f := range findings {
		if !f.Advisory && f.Severity > detection.Info {
			return true
		}
	}
	return false
}
