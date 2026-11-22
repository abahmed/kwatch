package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// changeGap is how long after a change the first failure it is blamed
// for began, or nil when the failure did not follow the change or a
// time is unknown.
func changeGap(change inventory.Change, onset time.Time) *time.Duration {
	if onset.IsZero() || change.At.IsZero() || onset.Before(change.At) {
		return nil
	}
	gap := onset.Sub(change.At)
	return &gap
}
