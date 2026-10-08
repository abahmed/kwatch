package scenarios

import (
	"context"
	"strings"
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
	return servicesSync(t, store, true, true)
}

// refuseCreates makes the API server refuse pods because it cannot call
// the first webhook: a dead backend pages only once a request is refused.
func refuseCreates(c *cluster) {
	w := c.deployment("shop", "orders", "registry.example.com/o:2", 2)
	c.list(w.objects())
	message := "Internal error occurred: failed calling webhook " +
		"\"a.policy.example.com\": failed to call webhook: Post " +
		"\"https://" + c.n("policy-svc") + "." + c.n("policy") +
		".svc:443/validate?timeout=10s\": service \"" +
		c.n("policy-svc") + "\" not found"
	admissionFailedCreates(c, []*workload{w}, message, 3, 30*time.Second)
}

// servicesSync replays a cluster with fail-closed webhooks whose Service
// is missing, and, when refused, a create the first of them refuses.
// With late set the Service kind syncs after the webhooks were first
// evaluated; otherwise every kind is synced from the start.
func servicesSync(
	t *testing.T, store *persistingStore, late, refused bool,
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
	c.after(time.Minute)
	if refused {
		refuseCreates(c)
	}
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
		// The refused create names a webhook, so the webhook is the
		// root; without it the missing Service would be.
		found = found || strings.HasPrefix(d.Incident.Root.Name, "policy-")
	}
	if !found {
		t.Errorf("no incident for the missing backend: %s",
			describeDecisions(result))
	}
}

// After a restart the restored incident of a missing webhook backend
// must be found again, not resolved as healthy. No request was refused:
// the incident is told in the startup summary and kept for the digest.
func TestRestartKeepsMissingBackendIncidentWhenServicesSyncLate(
	t *testing.T,
) {
	store := newPersistingStore()
	servicesSync(t, store, false, false)
	if store.records() == 0 {
		t.Fatal("the first session kept no incident")
	}

	second := servicesSync(t, store, true, false)

	for _, d := range second.Decisions {
		if d.Action == incident.Resolve {
			t.Errorf("resolved after restart: %s", describeDecisions(second))
		}
	}
}
