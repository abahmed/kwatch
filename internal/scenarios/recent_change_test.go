package scenarios

import (
	"strings"
	"testing"
)

// A failure nothing explains names the edit made next to it; the twin,
// whose only edit is older than the window, names nothing.
func TestUnknownCauseNamesOnlyRecentChanges(t *testing.T) {
	recent := scenarioNotes(t, "unknown-cause-recent-change")
	if !strings.Contains(recent, "In the last 30 minutes in shop: "+
		"bob changed config map feature-flags at 10:01.") {
		t.Errorf("the recent edit is not named:\n%s", recent)
	}
	old := scenarioNotes(t, "unknown-cause-old-change")
	if strings.Contains(old, "In the last 30 minutes") {
		t.Errorf("an old edit must not be named:\n%s", old)
	}
}
