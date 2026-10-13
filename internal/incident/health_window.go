package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// HealthWindow is the stretch between the last time the root was healthy
// and the first failure of the incident, with the changes recorded near
// the root inside it. It is a fact about what happened: the changes are
// those the model saw, and nothing says they caused anything.
type HealthWindow struct {
	// Healthy is the time of the root's last healthy mark before the
	// failure, and Failed is when the first failure began.
	Healthy, Failed time.Time
	// Revision is the revision the root was at when it was last
	// healthy, when it was recorded.
	Revision string
	// Changes are the changes near the root made in the window, nearest
	// to the root first, at most MaxRecentChanges.
	Changes []inventory.Change
}

// WindowHorizon is how far back a health window may begin. It is the
// span the model keeps changes for, so a window never reaches past what
// the change history can answer.
const WindowHorizon = inventory.ChangeRetention

// WindowOf returns the health window of p as of now, or nil. It needs a
// healthy mark of the root and a known failure start, the mark inside
// WindowHorizon and before the failure, and at least one change in
// between: without them nothing is said. The change that caused the
// incident is left out, because the message already names it.
func WindowOf(
	model inventory.HistoryReader, p Incident, now time.Time,
) *HealthWindow {
	failed := p.Onset()
	if model == nil || failed.IsZero() {
		return nil
	}
	mark, ok := lastHealthyBefore(model, p.Root, failed, now)
	if !ok {
		return nil
	}
	var blamed *inventory.Change
	if p.Cause != nil {
		blamed = p.Cause.Change
	}
	var changes []inventory.Change
	for _, change := range rankedBetween(model, p.Root, mark.At,
		failed.Add(onsetSlack), nearNamespace) {
		if blamed == nil || change.Entity != blamed.Entity ||
			!change.At.Equal(blamed.At) {
			changes = append(changes, change)
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return &HealthWindow{Healthy: mark.At, Failed: failed,
		Revision: mark.Revision, Changes: changes}
}

// lastHealthyBefore is id's latest healthy mark within the horizon that
// came before failed.
func lastHealthyBefore(
	model inventory.HistoryReader, id inventory.EntityID,
	failed, now time.Time,
) (inventory.HealthMark, bool) {
	var last inventory.HealthMark
	found := false
	for _, mark := range model.HealthMarks(id, now.Add(-WindowHorizon)) {
		if !mark.Failing && mark.At.Before(failed) &&
			(!found || mark.At.After(last.At)) {
			last, found = mark, true
		}
	}
	return last, found
}
