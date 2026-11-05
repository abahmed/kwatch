package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rolloutStuck makes web's ready replicas cover its desired count again
// with a pod of the previous ReplicaSet (the previous version serves)
// while its crashing pod keeps failing.
func (r *impactRig) rolloutStuck(ready float64) {
	container := inventory.CoreID(kube.KindContainer, r.pod.Namespace,
		r.pod.Name+"/app")
	r.observeWith(r.deploy, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(1),
		kube.AttrReadyReplicas: inventory.Number(ready),
	})
	r.observeWith(r.pod, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	r.observeWith(container, map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(6)})
	r.relate(container, inventory.PartOf, r.pod)
	// The previous version's pod, of the previous ReplicaSet.
	old := inventory.CoreID(kube.KindPod, r.pod.Namespace, "web-old-0")
	oldSet := inventory.CoreID(kube.KindReplicaSet, r.pod.Namespace,
		"web-old-rs")
	r.relate(old, inventory.OwnedBy, oldSet)
	r.relate(oldSet, inventory.OwnedBy, r.deploy)
	r.observeWith(old, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
}

// A page for a real outage falls back to a notification once the
// previous version serves again and only the new rollout fails: users
// are served, so it is not "down" any more.
func TestPageEasesWhenThePreviousVersionServes(t *testing.T) {
	r := newImpactRig(t, "LoadBalancer", false, 1, 0, -1, 0)
	if got := r.crash(); got.Tier != Page {
		t.Fatalf("tier = %v, want page while nothing serves", got.Tier)
	}
	r.tick(at(5 * time.Minute))
	r.rolloutStuck(1)
	r.raise(at(6*time.Minute),
		sig(r.pod, reasons.ReadinessProbeFailed, detection.Warning))
	r.tick(at(7 * time.Minute))
	got := r.only()
	if got.Tier != Notify || !got.PreviousServes {
		t.Fatalf("tier = %v, previous serves = %v; want notify, true",
			got.Tier, got.PreviousServes)
	}
}

// While nothing serves the page stays: the rollout is not the only
// thing wrong.
func TestPageStaysWhileNothingServes(t *testing.T) {
	r := newImpactRig(t, "LoadBalancer", false, 1, 0, -1, 0)
	r.crash()
	r.tick(at(5 * time.Minute))
	r.rolloutStuck(0)
	r.raise(at(6*time.Minute),
		sig(r.pod, reasons.ReadinessProbeFailed, detection.Warning))
	r.tick(at(7 * time.Minute))
	if got := r.only(); got.Tier != Page || got.PreviousServes {
		t.Fatalf("tier = %v, previous serves = %v; want page, false",
			got.Tier, got.PreviousServes)
	}
}
