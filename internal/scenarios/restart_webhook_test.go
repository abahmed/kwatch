package scenarios

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

// lateServices is the production behaviour of the informers on a start:
// a kind reads as synced only once its objects arrived, and the small
// webhook list arrives before the Services. The test model has a Service
// kind synced exactly when it holds a Service.
func lateServices(t *testing.T, store *persistingStore) replay.Result {
	return servicesSync(t, store, true)
}

// servicesSync replays a cluster with fail-closed webhooks whose Service
// is missing. With late set the Service kind syncs after the webhooks
// were first evaluated; otherwise every kind is synced from the start.
func servicesSync(
	t *testing.T, store *persistingStore, late bool,
) replay.Result {
	t.Helper()
	deps := newDependencies()
	deps.Store = store
	model := deps.Model
	synced := func(kind inventory.Kind) bool {
		return !late || kind != kube.KindService ||
			len(model.Entities(kube.KindService)) > 0
	}
	deps.Synced = synced
	deps.Detectors = detection.NewRegistry(synced, appDetectors()...)
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"))
	for _, n := range []string{"a", "b"} {
		c.list(clusterValidatingHook(c, "policy-"+n,
			n+".policy.example.com", "policy", "policy-svc"))
	}
	c.after(2 * time.Second)
	c.list(clusterService(c, "other", "other-svc", 443))
	c.after(20 * time.Minute)
	log := c.log()
	result, err := replay.Run(context.Background(), log, deps,
		replay.Options{SyncAt: log.Start.Add(3 * time.Second),
			Tail: 40 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// A webhook whose Service does not exist is reported even when the
// webhook list reached kwatch before the Service list: the webhooks are
// evaluated again once the Service kind is synced.
func TestMissingBackendIsFoundWhenServicesSyncLate(t *testing.T) {
	result := lateServices(t, newPersistingStore())

	if len(announcedIDs(result)) == 0 {
		t.Fatalf("nothing announced: %s", describeDecisions(result))
	}
	found := false
	for _, d := range result.Decisions {
		found = found || d.Incident.Root.Name == "policy-svc"
	}
	if !found {
		t.Errorf("no incident for the missing Service: %s",
			describeDecisions(result))
	}
}

// After a restart the restored incident of a missing webhook backend
// must be found again, not resolved as healthy.
func TestRestartKeepsMissingBackendIncidentWhenServicesSyncLate(
	t *testing.T,
) {
	store := newPersistingStore()
	first := servicesSync(t, store, false)
	if len(announcedIDs(first)) == 0 {
		t.Fatal("the first session announced nothing")
	}

	second := lateServices(t, store)

	for _, d := range second.Decisions {
		if d.Action == incident.Resolve {
			t.Errorf("resolved after restart: %s", describeDecisions(second))
		}
	}
}
