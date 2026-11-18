package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// servedRun is a run that sees the payments Deployment's newest
// ReplicaSet with a ready replica, through the recorder, with a writer
// on store.
func servedRun(
	t *testing.T, store IncidentStore,
) (*historyRecorder, *historyWriter) {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	w := newHistoryWriter(store.(HistoryStore), model, wallTimer,
		&workerStats{})
	h := &historyRecorder{model: model, writer: w,
		now: func() time.Time { return restartBase }}
	rs := kube.EntityFor("apps/v1", "ReplicaSet", "shop", "payments-a")
	for _, o := range []inventory.Observation{
		{Kind: inventory.Observed, Entity: restartDeploy},
		{Kind: inventory.Observed, Entity: rs,
			Attributes: map[string]inventory.Value{
				kube.AttrRevision:      inventory.Text("1"),
				kube.AttrReadyReplicas: inventory.Number(2)}},
		{Kind: inventory.Related, Entity: rs, Relation: inventory.OwnedBy,
			Targets: []inventory.EntityID{restartDeploy}},
	} {
		o.Source, o.At = "test", restartBase.Add(-time.Hour)
		_, err := model.Apply(o)
		require.NoError(t, err)
		h.served(o)
	}
	return h, w
}

func restoredServed(t *testing.T, store IncidentStore) []string {
	t.Helper()
	h := relaunchedHarness(t, store)
	var names []string
	for _, s := range h.engine.deps.Model.ServedRevisions(restartDeploy,
		time.Time{}) {
		names = append(names, s.Name)
	}
	return names
}

// The revisions a workload served survive a restart, beside its
// last-healthy mark: a mark saved later keeps the served revisions and
// the other way round.
func TestServedRevisionsSurviveARestart(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	_, w := servedRun(t, store)
	w.flush()
	w.addHealthy(restartDeploy, inventory.HealthMark{
		At: restartBase.Add(-time.Hour)})
	w.flush()

	assert.Equal(t, []string{"payments-a"}, restoredServed(t, store))
	marks := relaunchedHarness(t, store).engine.deps.Model.HealthMarks(
		restartDeploy, time.Time{})
	assert.Len(t, marks, 1, "the healthy mark is kept beside them")
}

// Deleting the workload forgets what it served on disk too; its
// healthy mark stays.
func TestServedRevisionsForgottenWithTheWorkload(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	h, w := servedRun(t, store)
	w.addHealthy(restartDeploy, inventory.HealthMark{
		At: restartBase.Add(-time.Hour)})
	w.flush()

	h.served(inventory.Observation{Kind: inventory.Changed,
		Entity: restartDeploy, At: restartBase,
		Change: inventory.Change{Deleted: true}})
	w.flush()

	assert.Empty(t, restoredServed(t, store))
	assert.Empty(t, h.model.ServedRevisions(restartDeploy, time.Time{}))
	marks := relaunchedHarness(t, store).engine.deps.Model.HealthMarks(
		restartDeploy, time.Time{})
	assert.Len(t, marks, 1)
}

// A record from an older version, with no served revisions, loads; a
// record with more than the bound restores only the most recent.
func TestServedRevisionsRestoreIsBounded(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	var served []inventory.ServedRevision
	for i := range inventory.MaxServedRevisions + 3 {
		served = append(served, inventory.ServedRevision{
			Name: fmt.Sprintf("payments-%d", i),
			At:   restartBase.Add(time.Duration(i-10) * time.Hour)})
	}
	require.NoError(t, store.(healthyStore).SaveHealthy(
		map[string]healthyRecord{restartDeploy.String(): {
			Mark: inventory.HealthMark{At: restartBase}, Served: served}}))

	got := restoredServed(t, store)

	assert.Len(t, got, inventory.MaxServedRevisions)
	assert.Equal(t, "payments-7", got[len(got)-1])
}

// A served revision alone keeps a record on disk for the served
// retention, past the healthy one.
func TestHealthyRecordExpiresWithItsNewestEvidence(t *testing.T) {
	r := healthyRecord{Mark: inventory.HealthMark{At: restartBase},
		Served: []inventory.ServedRevision{{Name: "a",
			At: restartBase.Add(time.Hour)}}}
	assert.Equal(t, restartBase.Add(time.Hour+inventory.ServedRetention),
		r.expires())
	assert.Equal(t, restartBase.Add(healthyRetention),
		healthyRecord{Mark: r.Mark}.expires())
}
