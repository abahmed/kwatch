package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// servedSightedAt is when the saved run last saw payments-7f serve.
var servedSightedAt = restartBase.Add(-2 * time.Hour)

// saveServed stores the payments Deployment's memory of payments-7f,
// the ReplicaSet name its template gives it, serving.
func saveServed(t *testing.T, store IncidentStore) {
	t.Helper()
	require.NoError(t, store.(healthyStore).SaveHealthy(
		map[string]healthyRecord{restartDeploy.String(): {
			Served: []inventory.ServedRevision{{
				Name: "payments-7f", At: servedSightedAt}}}}))
}

// relaunchServed restarts on store with the payments Deployment
// created at created, or absent (zero created), lists it in full when
// listed, runs one step and flushes the writer. It returns the
// revisions the model remembers payments serving.
func relaunchServed(
	t *testing.T, store IncidentStore, created time.Time, listed bool,
) []inventory.ServedRevision {
	t.Helper()
	h := newHarnessWith(t, restartBase, func(d *Dependencies) {
		d.Store = store
		d.Synced = func(inventory.Kind) bool { return listed }
	})
	history := h.engine.storage.history
	history.start()
	t.Cleanup(func() { history.askStop() })
	if !created.IsZero() {
		d, rs := deployment("payments")
		d.CreationTimestamp = metav1.NewTime(created)
		h.add(kube.DeploymentSchema(), d)
		h.add(kube.ReplicaSetSchema(), rs)
	}
	h.run(h.now, time.Second)
	history.writer.flush()
	return h.engine.deps.Model.ServedRevisions(restartDeploy, time.Time{})
}

// A workload deleted while kwatch was down is absent from the full
// list: its restored memory goes, in the model and on disk. Before its
// kind is fully listed, absence says nothing and the memory stays.
func TestRestoredServedOfAWorkloadGoneWhileDown(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	saveServed(t, store)

	assert.Len(t, relaunchServed(t, store, time.Time{}, false), 1,
		"kind not listed yet: memory kept")
	assert.Empty(t, relaunchServed(t, store, time.Time{}, true),
		"absent after the full list")
	assert.Empty(t, restoredServed(t, store), "forgotten on disk")
}

// A workload deleted and created again while kwatch was down has the
// same ReplicaSet name for an unchanged template; created after the
// remembered sighting, the old object's memory goes. The memory of a
// workload created before it stays.
func TestRestoredServedOfAWorkloadRecreatedWhileDown(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	saveServed(t, store)

	kept := relaunchServed(t, store, servedSightedAt.Add(-time.Hour),
		true)
	require.Len(t, kept, 1, "created before the sighting: kept")
	assert.Equal(t, "payments-7f", kept[0].Name)

	assert.Empty(t, relaunchServed(t, store,
		servedSightedAt.Add(time.Hour), true), "recreated after it")
	assert.Empty(t, restoredServed(t, store), "forgotten on disk")
}
