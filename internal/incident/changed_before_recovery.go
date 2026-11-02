package incident

import (
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Changes named next to a recovery: those the incident linked in the
// BeforeRecovery before it began to recover, newest first, at most
// MaxBeforeRecovery. Messages and the dashboard name them as what
// changed, never as what fixed it.
const (
	BeforeRecovery    = 15 * time.Minute
	MaxBeforeRecovery = 2
)

// ChangesBeforeRecovery are the linked changes of p made in the
// BeforeRecovery before it recovered and after its failures began, the
// fixing change fix (nil when none) and changes Kubernetes made by
// itself left out, newest first and at most MaxBeforeRecovery of them.
func ChangesBeforeRecovery(
	p Incident, fix *inventory.Change,
) []inventory.Change {
	end := p.RecoveredAt()
	from := end.Add(-BeforeRecovery)
	if onset := p.Onset(); onset.After(from) {
		from = onset
	}
	var out []inventory.Change
	for _, c := range p.Journal.Changes {
		if c.At.After(from) && !c.At.After(end) && !SystemChange(c) &&
			!sameAsFix(fix, c) {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b inventory.Change) int {
		return b.At.Compare(a.At)
	})
	return out[:min(len(out), MaxBeforeRecovery)]
}

// sameAsFix reports the change fix names.
func sameAsFix(fix *inventory.Change, c inventory.Change) bool {
	return fix != nil && fix.Entity == c.Entity && fix.At.Equal(c.At)
}

// SystemChange reports a change Kubernetes made by itself: a node's
// taints alone, which follow its conditions.
func SystemChange(change inventory.Change) bool {
	if change.Entity.Kind != kube.KindNode || change.Deleted ||
		len(change.Fields) == 0 {
		return false
	}
	for _, field := range change.Fields {
		if lastPathSegment(field.Path) != "taints" {
			return false
		}
	}
	return true
}

// lastPathSegment is the last part of a field path: "spec.taints" is
// "taints".
func lastPathSegment(path string) string {
	if i := strings.LastIndexAny(path, ".]"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}
