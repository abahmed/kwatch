package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const quotaFailedCreate = `pods "api-x" is forbidden: exceeded quota: ` +
	`compute, requested: cpu=1`

// shortReplicaSet is a ReplicaSet that wants desired pods and has ready
// of them, with a FailedCreate event at t0.
func shortReplicaSet(desired, ready float64,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kube.KindReplicaSet, "shop", "api-1")
	observeEntity(m, id, t0, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(desired),
		kube.AttrReadyReplicas: inventory.Number(ready),
	})
	noteEntity(m, id, "FailedCreate", quotaFailedCreate, 1, t0)
	return m, id
}

func eventsAt(m *inventory.Model, id inventory.EntityID, after time.Duration,
) []string {
	ctx := testDetectorContext(m, t0.Add(after))
	return findingReasons(Event{}.Detect(ctx, entityOf(m, id)))
}

// The controller retries a refused create with a backoff that can grow
// past EventWindow, so while it is still short of pods and has not yet
// had its next try the failure is still current.
func TestEventFailedCreateOutlivesTheWindowWhileTheControllerIsShort(
	t *testing.T,
) {
	m, id := shortReplicaSet(2, 0)

	assert.Contains(t, eventsAt(m, id, EventWindow-time.Minute),
		"FailedCreate")
	assert.Contains(t, eventsAt(m, id, EventWindow+2*time.Minute),
		"FailedCreate", "the controller's backoff is still running")
	assert.Empty(t, eventsAt(m, id, 2*EventWindow+time.Minute),
		"the controller has stopped trying")
}

func TestEventFailedCreateEndsWithTheWindowWhenTheControllerCaughtUp(
	t *testing.T,
) {
	m, id := shortReplicaSet(2, 2)

	assert.Empty(t, eventsAt(m, id, EventWindow+2*time.Minute))
}

func TestEventFailedCreateAsksForARecheckWhenTheBackoffEnds(t *testing.T) {
	m, id := shortReplicaSet(2, 0)
	now := t0.Add(EventWindow + 2*time.Minute)

	got := evaluate(Event{}, m, now, id, nil)

	require.Len(t, got.Findings, 1)
	assert.Equal(t, EventWindow-2*time.Minute+time.Nanosecond,
		got.RecheckAfter)
}
