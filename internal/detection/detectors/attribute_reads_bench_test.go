package detectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// busyNodeModel is one tainted node running pods pods of a DaemonSet,
// each with a phase, readiness, requests and a dozen other attributes,
// as a real pod has: what a per-pod snapshot copies.
func busyNodeModel(pods int) (*inventory.Model, inventory.EntityID,
	inventory.EntityID) {
	model := newTestModel()
	at := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	node := inventory.CoreID(kube.KindNode, "", "n1")
	ds := inventory.CoreID(kube.KindDaemonSet, "ops", "agent")
	apply := func(id inventory.EntityID, attrs map[string]inventory.Value) {
		model.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", At: at, Entity: id, Attributes: attrs})
	}
	relate := func(id inventory.EntityID, relation inventory.RelationType,
		to inventory.EntityID) {
		model.Apply(inventory.Observation{Kind: inventory.Related,
			Source: "test", At: at, Entity: id, Relation: relation,
			Targets: []inventory.EntityID{to}})
	}
	apply(node, map[string]inventory.Value{
		kube.AttrTaints: inventory.Text("dedicated=gpu:NoSchedule")})
	apply(ds, map[string]inventory.Value{})
	for i := 0; i < pods; i++ {
		pod := inventory.CoreID(kube.KindPod, "ops", fmt.Sprintf("p%d", i))
		attrs := map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
			kube.AttrReady: inventory.Bool(i%2 == 0),
		}
		for j := 0; j < 12; j++ {
			attrs[fmt.Sprintf("extra.%d", j)] = inventory.Text("value")
		}
		apply(pod, attrs)
		relate(pod, inventory.RunsOn, node)
		relate(pod, inventory.OwnedBy, ds)
	}
	return model, node, ds
}

// BenchmarkNodePodReads measures the per-pass reads of the pods on one
// node: which still run (requestedFit, requestedOn) and which are not
// ready on a tainted node (daemonSetGaps). Each reads one attribute
// per pod, so it allocates no entity snapshot.
func BenchmarkNodePodReads(b *testing.B) {
	model, node, ds := busyNodeModel(50)
	daemonSet, _ := model.Entity(ds)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestedFit(model, node, resPods)
		requestedOn(model, node, kube.AttrCPUReq)
		daemonSetGaps(model, daemonSet)
	}
}

// TestNodePodReadsAllocateNoSnapshots bounds what one pass allocates
// per pod. Copying each pod to read one attribute cost about five
// allocations per pod (requestedFit) and seven (daemonSetGaps); a
// single attribute read costs none, leaving the relation lookups.
func TestNodePodReadsAllocateNoSnapshots(t *testing.T) {
	const pods = 50
	model, node, ds := busyNodeModel(pods)
	daemonSet, _ := model.Entity(ds)
	// The relation lookup itself allocates; the reads must add nothing.
	lookup := testing.AllocsPerRun(20, func() {
		model.Related(node, inventory.RunsOn, inventory.Incoming)
	})
	fit := testing.AllocsPerRun(20, func() {
		requestedFit(model, node, resPods)
	})
	if fit > lookup+1 {
		t.Errorf("requestedFit allocates %.0f times per pass over %d "+
			"pods, want at most %.0f", fit, pods, lookup+1)
	}
	gaps := testing.AllocsPerRun(20, func() {
		daemonSetGaps(model, daemonSet)
	})
	if gaps/pods > 3 {
		t.Errorf("daemonSetGaps allocates %.0f times per pass over %d "+
			"pods, want at most 3 per pod", gaps, pods)
	}
}
