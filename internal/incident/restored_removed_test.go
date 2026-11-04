package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// restoreRemoved restores an announced node incident into a manager
// whose fully listed model no longer holds the node, as when the node
// was deleted while kwatch was down, and ticks until it resolves.
func restoreRemoved(t *testing.T, reasons []string) Incident {
	t.Helper()
	old := newRig(t, Config{})
	node := sig(entity(kube.KindNode, "worker-1"), "NodeNotReady",
		detection.Critical)
	announced(t, old, node)
	records := old.m.Export()
	records[0].RootReasons = reasons
	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(time.Hour))
	// An empty snapshot gives the manager the fresh, synced model.
	fresh.m.Apply(fresh.snapshot(at(time.Hour)), nil, nil)
	for now := at(time.Hour); now.Before(at(3 * time.Hour)); {
		ds, wake := fresh.m.Tick(now)
		for _, d := range ds {
			if d.Action == Resolve {
				return d.Incident
			}
		}
		require.NotZero(t, wake, "no wake-up pending")
		now = now.Add(wake)
	}
	t.Fatal("incident did not resolve")
	return Incident{}
}

// A root that failed by itself and is absent once the model is listed
// was deleted while kwatch was down: the resolve says it was removed.
func TestRestoredRootGoneWhileDownResolvesAsRemoved(t *testing.T) {
	got := restoreRemoved(t, []string{"NodeNotReady"})

	assert.Contains(t, got.Removed, got.Root)
}

// A root with no reason of its own may never have existed, so its
// absence is not claimed as a removal.
func TestRestoredRootWithoutOwnReasonIsNotClaimedRemoved(t *testing.T) {
	got := restoreRemoved(t, nil)

	assert.Empty(t, got.Removed)
}
