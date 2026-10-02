package detection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestTrackerIgnoresSummaryOnlyChanges(t *testing.T) {
	tr := NewTracker()
	id := inventory.EntityID{Kind: "node", Name: "n1"}
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := Finding{Entity: id, Reason: "NodeNotReady", Severity: Critical,
		Since: at, Summary: "Node is NotReady for 2m"}
	tr.Observe(id, []Finding{s})

	for _, summary := range []string{
		"Node is NotReady for 15m", "Node stopped reporting for 16m",
	} {
		s.Summary = summary
		assert.Empty(t, tr.Observe(id, []Finding{s}),
			"summary text alone is not a change")
		require.Len(t, tr.Active(id), 1)
		assert.Equal(t, summary, tr.Active(id)[0].Summary,
			"the latest summary is kept")
	}
}

func TestTrackerReportsModeAndHealthChanges(t *testing.T) {
	id := inventory.EntityID{Kind: "container", Name: "c"}
	base := Finding{Entity: id, Reason: "CrashLoopBackOff",
		Severity: Critical, Summary: "crashing"}
	cases := []struct {
		name   string
		mutate func(*Finding)
		want   bool
	}{
		{"identical", func(*Finding) {}, false},
		{"summary only", func(f *Finding) { f.Summary = "other" }, false},
		{"mode", func(f *Finding) { f.Mode = "CrashLoop.OOM" }, true},
		{"health", func(f *Finding) { f.Health = Unknown }, true},
		{"severity", func(f *Finding) { f.Severity = Warning }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker()
			raised := tr.Observe(id, []Finding{base})
			require.Len(t, raised, 1)
			assert.Equal(t, Raised, raised[0].Kind)
			assert.Equal(t, Failing, raised[0].Finding.Health)
			assert.Equal(t, "CrashLoop", string(raised[0].Finding.Mode))

			next := base
			tc.mutate(&next)
			got := tr.Observe(id, []Finding{next})
			if !tc.want {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, Changed, got[0].Kind)
		})
	}
}
