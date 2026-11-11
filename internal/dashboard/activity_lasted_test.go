package dashboard

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// A resolve says how long the issue went on, so a row can read
// "resolved after 12 min"; other events say nothing.
func TestActivityResolveCarriesHowLongItWent(t *testing.T) {
	done := testIncident("d", "web", -30*time.Minute)
	done.State, done.Resolved = incident.Resolved, before(18*time.Minute)
	got := buildActivity([]incident.Incident{done}, testEpoch, time.Hour,
		redact.Evidence, true)
	for _, e := range got.Events {
		want := int64(0)
		if e.Kind == activityResolved {
			want = 12 * 60
		}
		if e.Lasted != want {
			t.Errorf("%s lasted %d, want %d", e.Kind, e.Lasted, want)
		}
	}
	if len(got.Events) != 2 {
		t.Errorf("events = %v", activityLine(got))
	}
}
