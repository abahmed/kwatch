package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// workload wires pod -> replicaset -> deployment.
func workload(r *rig, name string) (pod, deploy inventory.EntityID) {
	pod = entity(kube.KindPod, name+"-0")
	rs := entity(kube.KindReplicaSet, name+"-rs")
	deploy = entity(kube.KindDeployment, name)
	r.relate(pod, inventory.OwnedBy, rs)
	r.relate(rs, inventory.OwnedBy, deploy)
	return pod, deploy
}

func TestManagerGroupsReplicasUnderTheirWorkload(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	other := entity(kube.KindPod, "web-1")
	r.relate(other, inventory.OwnedBy, entity(kube.KindReplicaSet, "web-rs"))

	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Warning),
		sig(other, reasons.CrashLoopBackOff, detection.Warning))

	got := r.only()
	if got.Root != deploy || len(got.Members) != 2 {
		t.Fatalf("want one deployment incident, got %+v", got)
	}
}

func TestManagerContainerFindingBelongsToPodWorkload(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	container := entity(kube.KindContainer, "web-0/app")
	r.relate(container, inventory.PartOf, pod)

	r.raise(at(0), sig(container, reasons.OOMKilled, detection.Warning))
	if got := r.only(); got.Root != deploy {
		t.Fatalf("root = %v, want %v", got.Root, deploy)
	}
}

func TestManagerAttributesSymptomToCauseRoot(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	node := entity(kube.KindNode, "n1")
	h := r.cause(pod, node, "node ran out of memory")

	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Warning))
	got := r.only()
	if got.Root != node || got.Cause == nil ||
		got.Cause.Summary != h.Summary {
		t.Fatalf("want node cause, got root=%v cause=%+v",
			got.Root, got.Cause)
	}
}

func TestManagerCauseInsideOwnWorkloadIsNotACause(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	r.cause(deploy, pod, "pods are crashing")
	r.sigs[pod] = []detection.Finding{
		sig(pod, reasons.CrashLoopBackOff, detection.Warning),
	}

	r.raise(at(0), sig(deploy, "Unavailable", detection.Critical))
	got := r.only()
	if got.Root != deploy || got.Cause != nil {
		t.Fatalf("want own root without cause, got %+v", got)
	}
}

// TestManagerInsideRowOfOwnWorkloadIsACause: a row that blames a part
// of the workload itself (here its memory limit) is stated, although
// it names no outside object and nothing changed.
func TestManagerInsideRowOfOwnWorkloadIsACause(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	c := r.cause(pod, deploy, "memory limit too low")
	c.Row = "memory-limit-too-low"
	r.rule.causes[pod] = c

	r.raise(at(0), sig(pod, reasons.OOMKilled, detection.Warning))
	got := r.only()
	if got.Root != deploy || got.Cause == nil ||
		got.Cause.Rule != "memory-limit-too-low" {
		t.Fatalf("want the limit stated as the cause, got %+v", got)
	}
}

// explainerFunc is an Explainer written inline by a test.
type explainerFunc func(
	explain.Snapshot, []inventory.EntityID,
) []explain.Area

func (f explainerFunc) Solve(
	s explain.Snapshot, dirty []inventory.EntityID,
) []explain.Area {
	return f(s, dirty)
}

func TestManagerPlacesEveryFailureOfASolvedArea(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	node := entity(kube.KindNode, "n1")
	r.m.explainer = explainerFunc(func(
		explain.Snapshot, []inventory.EntityID,
	) []explain.Area {
		both := []inventory.EntityID{deploy, pod}
		return []explain.Area{{Failures: both, Causes: []explain.Cause{{
			Root: node, Covers: both, Confidence: 0.9}}}}
	})
	// The Deployment's finding was raised earlier; the pod's raise
	// solves the area again and both move to the node.
	unavailable := sig(deploy, "Unavailable", detection.Critical)
	r.sigs[deploy] = []detection.Finding{unavailable}
	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Warning))

	got := r.only()
	if got.Root != node || len(got.Members) != 2 || got.Cause == nil {
		t.Fatalf("want one node incident with both findings, got %+v", got)
	}
}

func TestManagerChangeOfOwnWorkloadIsACause(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	c := r.cause(pod, deploy, "new image rolled out")
	c.Changes = []inventory.Change{{Entity: deploy, Revision: "7"}}
	r.rule.causes[pod] = c

	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Warning))
	got := r.only()
	if got.Root != deploy || got.Cause == nil || got.Cause.Change == nil ||
		got.Cause.Change.Revision != "7" {
		t.Fatalf("want change cause at deployment, got %+v", got)
	}
}

func TestManagerInformationalFindingBelongsToItsWorkload(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	// No area covers the finding: the explainer only explains failures.
	r.m.explainer = explainerFunc(func(
		explain.Snapshot, []inventory.EntityID,
	) []explain.Area {
		return nil
	})
	r.raise(at(0), sig(pod, reasons.ContainerCPUHigh, detection.Info))
	if got := r.only(); got.Root != deploy || len(got.Members) != 1 {
		t.Fatalf("want the workload's incident, got %+v", got)
	}
}

func TestManagerReattachesFindingWhenCauseIsRevised(t *testing.T) {
	r := newRig(t, Config{})
	pod := podSig("web")
	r.raise(at(0), pod)
	node := entity(kube.KindNode, "n1")
	r.cause(pod.Entity, node, "node down")
	r.apply(at(1), detection.Changed, pod)

	records := r.m.Export()
	if len(records) != 2 {
		t.Fatalf("want old and new incident, got %d", len(records))
	}
	for _, rec := range records {
		p := r.m.incidents[rec.ID]
		switch rec.Root {
		case node:
			if len(p.Members) != 1 || !hasNote(p, "cause revised") {
				t.Fatalf("node incident wrong: %+v", p)
			}
		default:
			if len(p.Members) != 0 || !hasNote(p, "cause revised") ||
				hasNote(p, "recovered") {
				t.Fatalf("old incident should lose member without "+
					"claiming recovery: %+v", p)
			}
		}
	}
}

func hasNote(p *Incident, prefix string) bool {
	for _, e := range p.Timeline {
		if len(e.Text) >= len(prefix) && e.Text[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func TestManagerClearingUnknownFindingIsIgnored(t *testing.T) {
	r := newRig(t, Config{})
	r.clear(at(0), podSig("ghost"))
	if len(r.m.Export()) != 0 {
		t.Fatal("clearing an unknown finding must not create an incident")
	}
}

func TestManagerImpactListsWorkloadServiceAndIngress(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	slice := entity(kube.KindEndpointSlice, "web-x")
	svc := entity(kube.KindService, "web")
	ingress := entity(kube.KindIngress, "web")
	r.relate(slice, inventory.RoutesTo, pod)
	r.relate(slice, inventory.Backs, svc)
	r.relate(ingress, inventory.RoutesTo, svc)

	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff,
		detection.Critical))
	got := r.only()
	if got.Root != deploy || pageRuleOf(&got) != "traffic-lost" ||
		got.Tier != Page {
		t.Fatalf("want user-facing page, got %+v", got)
	}
	if impactSize(&got) != 2 {
		t.Fatalf("impact size = %d, want 2 (%v)",
			impactSize(&got), got.Impact)
	}
}

func TestManagerDescribeIncludesNamespaceWhenSet(t *testing.T) {
	ns := inventory.CoreID(kube.KindNode, "", "n1")
	if got := describe(ns); got != "node n1" {
		t.Fatalf("describe = %q", got)
	}
	if got := describe(entity(kube.KindPod, "p")); got != "pod shop/p" {
		t.Fatalf("describe = %q", got)
	}
}
