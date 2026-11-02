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

// A restart over a digest-tier missing-target incident, where the
// Deployments reach kwatch after the autoscaler: for a moment only the
// controller's FailedGetScale words describe the failure (notify). That
// partial view is not news: the restart says nothing about the incident,
// and it ends where it was, at the digest tier.
func TestRestartWithLateDeploymentsKeepsMissingTargetQuiet(t *testing.T) {
	store := newPersistingStore()
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"))
	other := c.deployment("staging", "other", "registry.example.com/o:1", 1)
	c.list(other.objects())
	hpa := agedMissingTargetHPA(c)
	c.list(hpa)
	c.after(30 * time.Minute)
	log := c.log()
	first := restartReplay(t, log, store, replay.Options{
		SyncAt: log.Start, Tail: 30 * time.Minute})
	if got := hpaTargetMembers(first); !got["HPATargetMissing"] {
		t.Fatalf("first session members = %v, want HPATargetMissing", got)
	}

	restartAt := first.End.Add(restartGap)
	again := newCluster(scenarioStart, "")
	again.now = restartAt
	again.list(again.node("n1", "zone-a"))
	again.list(hpa)
	again.after(2 * time.Second)
	again.list(other.objects())
	again.after(20 * time.Minute)
	log = again.log()
	log.Start = restartAt.UTC()
	second := lateDeploymentsReplay(t, log, store, replay.Options{
		SyncAt: restartAt.Add(3 * time.Second), Tail: 30 * time.Minute})

	for _, d := range second.Decisions {
		if d.Incident.Root.Name == "thirdparty" &&
			d.Action != incident.Announce {
			t.Errorf("the restart told the incident: %s",
				describeDecisions(second))
		}
	}
	for _, p := range second.Incidents {
		if p.Root.Name == "thirdparty" && p.Tier != incident.Digest {
			t.Errorf("tier = %v, want digest", p.Tier)
		}
	}
}

// lateDeploymentsReplay replays a session from store in which the
// Deployment kind counts as synced only once one is listed.
func lateDeploymentsReplay(
	t *testing.T, log replay.Log, store *persistingStore,
	opts replay.Options,
) replay.Result {
	t.Helper()
	deps := newDependencies()
	model := deps.Model
	synced := func(kind inventory.Kind) bool {
		return kind != kube.KindDeployment ||
			len(model.Entities(kube.KindDeployment)) > 0
	}
	deps.Synced = synced
	deps.Detectors = detection.NewRegistry(synced, appDetectors()...)
	deps.Store = store
	result, err := replay.Run(context.Background(), log, deps, opts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
