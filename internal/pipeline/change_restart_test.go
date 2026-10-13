package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// restartBase is the store clock of openTempStore, so nothing saved at
// or before it counts as expired.
var restartBase = time.Unix(1_800_000_000, 0).UTC()

var restartDeploy = kube.EntityFor("apps/v1", "Deployment", "shop", "payments")

// savePreviousRun plays the run before a restart: the deployment was
// healthy at healthyAt, and bob changed its template at changeAt. The
// history writer persists both, as it does a second after they happen.
func savePreviousRun(
	t *testing.T, store IncidentStore, healthyAt, changeAt time.Time,
) {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	w := newHistoryWriter(store.(HistoryStore), model, wallTimer,
		&workerStats{})
	change := inventory.Change{
		Entity: restartDeploy, At: changeAt, Actor: "bob", Revision: "8",
		Fields: []inventory.FieldChange{
			{Path: "spec.template.spec.containers[app].image",
				Before: "app:2.2", After: "app:2.3"}},
	}
	_, err := model.Apply(inventory.Observation{Kind: inventory.Changed,
		Source: "test", At: changeAt, Entity: restartDeploy, Change: change})
	require.NoError(t, err)
	w.addChange(change)
	w.addHealthy(restartDeploy, inventory.HealthMark{At: healthyAt})
	w.flush()
}

// relaunchedHarness is a new run on the same store: an empty model and
// the history restored from the state file.
func relaunchedHarness(t *testing.T, store IncidentStore) *harness {
	t.Helper()
	h := newHarnessWith(t, restartBase, func(d *Dependencies) {
		d.Store = store
	})
	h.engine.storage.history.start()
	t.Cleanup(func() { h.engine.storage.history.askStop() })
	return h
}

// failAfterRestart makes the payments deployment fail 14 hours after
// the change, the way a nightly restart leaves it.
func failAfterRestart(h *harness) {
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("payments")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, crashingPod("payments-a", rs.Name, "n1",
		h.now.Add(10*time.Second)))
	h.run(h.now.Add(12*time.Minute), 10*time.Second)
}

func firstNote(t *testing.T, h *harness) string {
	t.Helper()
	require.NotEmpty(t, h.messages, "expected an announcement")
	return notification.Text(h.messages[0])
}

// A change made 14 hours before a failure, and a restart in between,
// still reaches the message with its actor, field and time, and the
// window from the last healthy mark names it.
func TestRestartKeepsChangeActorAndWindow(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	changeAt := restartBase.Add(-14 * time.Hour)
	savePreviousRun(t, store, restartBase.Add(-15*time.Hour), changeAt)

	h := relaunchedHarness(t, store)
	failAfterRestart(h)

	text := firstNote(t, h)
	assert.Contains(t, text, "(last healthy)")
	assert.Contains(t, text, "(first failure)")
	assert.Contains(t, text, "bob")
	assert.Contains(t, text, "app:2.3")
	assert.Contains(t, text, changeAt.Format("15:04"))
	assert.NotContains(t, text, "changed while kwatch was down")
	t.Log(text)
}

// Without a healthy mark nothing is known about when the deployment
// last worked, so no window is claimed.
func TestRestartWithoutHealthyMarkSaysNoWindow(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	savePreviousRun(t, store, restartBase.Add(-15*time.Hour),
		restartBase.Add(-14*time.Hour))
	require.NoError(t, store.(*diskIncidents).healthy().DeleteMany(
		[]string{healthyKeyPrefix + restartDeploy.String()}))

	h := relaunchedHarness(t, store)
	failAfterRestart(h)

	assert.False(t, strings.Contains(firstNote(t, h), "(last healthy)"))
}

// A change older than the day a restart reloads is not brought back, and
// a healthy mark that old is not claimed as a window start.
func TestRestartReloadsOnlyTheLastDay(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	savePreviousRun(t, store, restartBase.Add(-49*time.Hour),
		restartBase.Add(-48*time.Hour))

	h := relaunchedHarness(t, store)

	assert.Empty(t, h.engine.deps.Model.Changes(restartDeploy, time.Time{}))
	assert.Empty(t, h.engine.deps.Model.HealthMarks(restartDeploy,
		time.Time{}))
}
