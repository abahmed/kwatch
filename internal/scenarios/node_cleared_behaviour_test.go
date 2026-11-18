package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// A node stops explaining pods once its own problem is over and the
// failures are no longer its own: its incident resolves soon after,
// and the resolve does not call a workload that still fails healthy.
func TestClearedNodeStopsExplainingItsPods(t *testing.T) {
	result := replayNamed(t, "node-stall-clears-pods-still-unready")
	var nodeResolved time.Time
	for i, d := range result.Decisions {
		if d.Action != incident.Resolve ||
			d.Incident.Root.String() != "node//n1" {
			continue
		}
		nodeResolved = result.Times[i]
		text := notification.Text(result.Messages[i])
		if strings.Contains(text, "api in shop") &&
			strings.Contains(text, "healthy again") {
			t.Errorf("resolve calls a workload that still fails "+
				"healthy: %s", text)
		}
	}
	if nodeResolved.IsZero() {
		t.Fatal("the node's incident stayed open on pods it no longer " +
			"explains")
	}
	// The stall ends twelve minutes in; the resolve hold is minutes,
	// never hours.
	if limit := result.Times[0].Add(20 * time.Minute); nodeResolved.
		After(limit) {
		t.Errorf("node resolved at %s, after %s", nodeResolved, limit)
	}
}
