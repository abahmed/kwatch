package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/storage"
)

func templateChange(name string, at time.Time) inventory.Change {
	return inventory.Change{
		Entity: kube.EntityFor("apps/v1", "Deployment", "shop", name),
		At:     at, Observed: at.Add(time.Second), Actor: "argocd",
		App: "argocd/shop", Revision: "4", Cause: "bump api",
		Fields: []inventory.FieldChange{
			{Path: "spec.template", Before: "a", After: "b"}},
	}
}

func TestChangeStoreRoundTripKeepsWholeChange(t *testing.T) {
	store := NewIncidentStore(openTempStore(t)).(changeStore)
	want := templateChange("api", restartBase.Add(-time.Hour))

	require.NoError(t, store.AppendChanges(
		[]changeRecord{{Change: want, ChangeSet: "cs-1"}}))
	got, err := store.LoadChanges(time.Time{}, 10)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "cs-1", got[0].ChangeSet)
	assert.Equal(t, want.Entity, got[0].Change.Entity)
	assert.True(t, want.At.Equal(got[0].Change.At))
	assert.True(t, want.Observed.Equal(got[0].Change.Observed))
	assert.Equal(t, want.Fields, got[0].Change.Fields)
	assert.Equal(t, []string{"argocd", "argocd/shop", "4", "bump api"},
		[]string{got[0].Change.Actor, got[0].Change.App,
			got[0].Change.Revision, got[0].Change.Cause})
}

// The reload takes the newest records up to the limit, and none older
// than since.
func TestChangeStoreLoadIsBoundedByCountAndAge(t *testing.T) {
	store := NewIncidentStore(openTempStore(t)).(changeStore)
	var records []changeRecord
	for i := range 50 {
		at := restartBase.Add(time.Duration(i-50) * time.Hour)
		records = append(records, changeRecord{
			Change: templateChange(fmt.Sprintf("api-%02d", i), at)})
	}
	require.NoError(t, store.AppendChanges(records))

	newest, err := store.LoadChanges(time.Time{}, 5)
	require.NoError(t, err)
	require.Len(t, newest, 5)
	assert.Equal(t, "api-49", newest[4].Change.Entity.Name)
	assert.Equal(t, "api-45", newest[0].Change.Entity.Name)

	lastDay, err := store.LoadChanges(
		restartBase.Add(-inventory.ChangeRetention), 1000)
	require.NoError(t, err)
	assert.Len(t, lastDay, 24, "only the last 24 hours reload")
}

// A state file from before this feature has no change records and no
// healthy marks; records of a version this build does not know are
// skipped rather than misread.
func TestChangeStoreOldAndNewerStateFiles(t *testing.T) {
	state := openTempStore(t)
	store := NewIncidentStore(state)
	got, err := store.(changeStore).LoadChanges(time.Time{}, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
	healthy, err := store.(healthyStore).LoadHealthy(10)
	require.NoError(t, err)
	assert.Empty(t, healthy)

	future := changeRecord{V: recordVersion + 1,
		Change: templateChange("api", restartBase)}
	require.NoError(t, storage.ChangeLog[changeRecord](state).Append(
		changeStream, restartBase, future))
	got, err = store.(changeStore).LoadChanges(time.Time{}, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestHealthyStoreKeepsNewestAndExpires(t *testing.T) {
	store := NewIncidentStore(openTempStore(t)).(healthyStore)
	records := map[string]healthyRecord{}
	for i := range 5 {
		id := inventory.CoreID("deployment", "shop", fmt.Sprintf("w%d", i))
		records[id.String()] = healthyRecord{Mark: inventory.HealthMark{
			At:       restartBase.Add(-time.Duration(i) * time.Hour),
			Revision: "r"}}
	}
	require.NoError(t, store.SaveHealthy(records))

	got, err := store.LoadHealthy(2)

	require.NoError(t, err)
	assert.Len(t, got, 2)
	newest := inventory.CoreID("deployment", "shop", "w0").String()
	assert.Contains(t, got, newest)

	stale := map[string]healthyRecord{"old": {Mark: inventory.HealthMark{
		At: restartBase.Add(-healthyRetention - time.Hour)}}}
	require.NoError(t, store.SaveHealthy(stale))
	all, err := store.LoadHealthy(100)
	require.NoError(t, err)
	assert.NotContains(t, all, "old", "an expired record is not read")
}

func TestPersistsChangeKeepsEditsOfWorkloadsAndConfig(t *testing.T) {
	at := restartBase
	pod := inventory.Change{Entity: kube.EntityFor("v1", "Pod", "shop", "p"),
		At: at, Created: true}
	status := inventory.Change{Entity: restartDeploy, At: at}
	cfg := inventory.Change{Entity: kube.EntityFor(
		"v1", "ConfigMap", "shop", "cfg"), At: at,
		Fields: []inventory.FieldChange{{Path: "data.k"}}}

	assert.False(t, persistsChange(pod), "pod churn is not kept")
	assert.False(t, persistsChange(status), "a status write is not kept")
	assert.True(t, persistsChange(templateChange("api", at)))
	assert.True(t, persistsChange(cfg))
}

// The writer queue is bounded: when the store lags, the oldest queued
// changes are dropped first.
func TestWriterQueuesAtMostMaxPendingChanges(t *testing.T) {
	w := newHistoryWriter(nil, inventory.NewModel(inventory.Options{}),
		wallTimer, &workerStats{})
	for i := range maxPendingChanges + 100 {
		w.addChange(templateChange("api",
			restartBase.Add(time.Duration(i)*time.Second)))
	}

	require.Len(t, w.changes, maxPendingChanges)
	assert.True(t, w.changes[0].At.Equal(restartBase.Add(100*time.Second)))
}
