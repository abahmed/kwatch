package announce

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// restartRig restarts kwatch over the same two announced incidents,
// carrying the startup marker from one start to the next.
type restartRig struct {
	t      *testing.T
	marker StartupState
	found  bool
	issues string
}

// start is one kwatch start at now: the incidents come back failing,
// the grace ends, and the restored listing runs. It returns what was
// sent.
func (r *restartRig) start(
	now time.Time, restart Restart,
) []notification.Message {
	r.t.Helper()
	m := incident.NewManager(incident.Config{}, explain.NewSolver())
	var records []incident.Record
	var findings []detection.Transition
	snapshot := explain.Snapshot{Now: now,
		Model:    inventory.NewModel(inventory.Options{}),
		Findings: map[inventory.EntityID][]detection.Finding{}}
	for _, name := range []string{"api", "web"} {
		root := inventory.CoreID(kube.KindPod, "shop", name)
		records = append(records, incident.Record{ID: "inc-" + name,
			Root: root, Tier: incident.Notify, State: incident.Open,
			Opened:    now.Add(-2 * time.Hour),
			Announced: now.Add(-2 * time.Hour)})
		f := detection.Finding{Entity: root, Reason: "CrashLoopBackOff",
			Severity: detection.Critical, Since: now.Add(-2 * time.Hour)}
		snapshot.Findings[root] = []detection.Finding{f}
		findings = append(findings,
			detection.Transition{Kind: detection.Raised, Finding: f})
	}
	m.Restore(records, now)
	m.Apply(snapshot, nil, findings)
	writer := compose.Writer{}
	if r.issues != "" {
		writer.Dashboard = func() string { return r.issues }
	}
	var sent []notification.Message
	c := New(Env{Incidents: m, Messages: writer, Write: writer.Write,
		Restart: restart,
		SaveStartup: func(s StartupState) {
			r.marker, r.found = s.Clone(), true
		},
		Sink: func(_ context.Context, _ incident.Decision,
			msg notification.Message) {
			sent = append(sent, msg)
		},
	})
	marker := r.marker
	marker.Complete = true
	c.Startup.Restore(marker, true, len(records), now)
	c.ListRestored(context.Background(), now)
	return sent
}

var apiLost = Restart{Reason: "api_unavailable", Down: 40 * time.Second}

// A restart soon after a listing does not list the same incidents
// again: one short note says kwatch restarted, why, and how many are
// still open; the same note is not repeated.
func TestRestoredListingIsNotRepeatedByQuickRestarts(t *testing.T) {
	r := &restartRig{t: t, issues: "https://k.example.com"}

	first := r.start(foldNow, apiLost)
	require.Len(t, first, 1)
	assert.Contains(t, first[0].Title, "still failing")
	require.NotNil(t, r.marker.Restored)
	assert.ElementsMatch(t, []string{"inc-api", "inc-web"},
		r.marker.Restored.IDs)

	second := r.start(foldNow.Add(10*time.Minute),
		Restart{At: foldNow.Add(9 * time.Minute),
			Reason: "api_unavailable", Down: 40 * time.Second})
	require.Len(t, second, 1)
	assert.Equal(t, "kwatch restarted at 08:40 UTC (it had lost the "+
		"API server, down 40s). 2 problems from before are still "+
		"open; see the dashboard: https://k.example.com/issues",
		second[0].Title)
	require.Len(t, second[0].Doc, 1)
	assert.NotNil(t, second[0].Listed, "the audit names the incidents")

	assert.Empty(t, r.start(foldNow.Add(20*time.Minute), apiLost),
		"the same note is not sent twice")
	other := r.start(foldNow.Add(30*time.Minute),
		Restart{Reason: "oom_killed"})
	require.Len(t, other, 1, "a new restart reason is news")
	assert.Contains(t, other[0].Title, "(it ran out of memory)")
}

// Past RestoredRelist the restored incidents are listed in full again.
func TestRestoredListingReturnsAfterTheRelistWindow(t *testing.T) {
	r := &restartRig{t: t}
	require.Len(t, r.start(foldNow, apiLost), 1)

	later := r.start(foldNow.Add(RestoredRelist), apiLost)

	require.Len(t, later, 1)
	assert.Contains(t, later[0].Title, "still failing")
}

func TestRestartNoteTextWithoutDashboardOrReason(t *testing.T) {
	got := restartNoteText(Restart{}, 1, foldNow, "")

	assert.Equal(t, "kwatch restarted at 08:31 UTC. 1 problem from "+
		"before is still open, as listed after an earlier restart.", got)
}

// The marker gained Restored later: a marker written before it loads,
// and a marker with it survives the store's JSON.
func TestStartupStateRestoredMarkRoundTrips(t *testing.T) {
	var old StartupState
	require.NoError(t, json.Unmarshal([]byte(`{"Complete":true}`), &old))
	assert.Nil(t, old.Restored)

	in := StartupState{Complete: true, Restored: &RestoredMark{
		At: foldNow, IDs: []string{"a"}, NoteAt: foldNow, NoteKey: "k"}}
	raw, err := json.Marshal(in)
	require.NoError(t, err)
	var out StartupState
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, in, out)

	clone := in.Clone()
	clone.Restored.IDs[0] = "changed"
	assert.Equal(t, "a", in.Restored.IDs[0], "Clone copies the mark")
}
