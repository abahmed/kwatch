package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// stuckSecret puts a Secret that was deleted at t0 into m.
func stuckSecret(m *inventory.Model) inventory.EntityID {
	id := newID(kube.KindSecret, "ops", "old-token")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0),
		kube.AttrFinalizers:    inventory.Text("example.com/cleanup"),
	})
	return id
}

func stuckSeverity(
	t *testing.T, m *inventory.Model, id inventory.EntityID,
	age time.Duration,
) detection.Severity {
	t.Helper()
	got := evaluate(Generic{}, m, t0.Add(age), id, nil).Findings
	require.Len(t, got, 1)
	return got[0].Severity
}

// A deletion stuck for days that nothing uses is housekeeping.
func TestStuckDeletionNothingUsesIsALeftover(t *testing.T) {
	m := newTestModel()
	id := stuckSecret(m)

	assert.Equal(t, detection.Warning,
		stuckSeverity(t, m, id, 2*time.Hour), "fresh: someone waits")
	assert.Equal(t, detection.Info,
		stuckSeverity(t, m, id, DefaultLeftoverAge+time.Minute))
}

// A pod that still uses the object keeps the deletion a warning.
func TestStuckDeletionUsedByPodStaysAWarning(t *testing.T) {
	m := newTestModel()
	id := stuckSecret(m)
	pod := newID(kube.KindPod, "ops", "api-0")
	put(m, pod, t0, nil)
	link(m, pod, inventory.References, id)

	assert.Equal(t, detection.Warning,
		stuckSeverity(t, m, id, 3*DefaultLeftoverAge))
}

// A child that is not being deleted keeps its owner's deletion live;
// a child that is being deleted too does not.
func TestStuckDeletionOwnedByChildrenIsLiveOnlyWhileTheyLive(t *testing.T) {
	m := newTestModel()
	id := stuckSecret(m)
	child := newID(kube.KindConfigMap, "ops", "child")
	put(m, child, t0, nil)
	link(m, child, inventory.OwnedBy, id)
	assert.Equal(t, detection.Warning,
		stuckSeverity(t, m, id, 3*DefaultLeftoverAge))

	put(m, child, t0, map[string]inventory.Value{
		kube.AttrDeleting: inventory.Bool(true)})
	assert.Equal(t, detection.Info,
		stuckSeverity(t, m, id, 3*DefaultLeftoverAge))
}
