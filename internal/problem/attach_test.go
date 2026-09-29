package problem

import (
	"testing"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// workload wires pod -> replicaset -> deployment.
func workload(r *rig, name string) (pod, deploy knowledge.EntityID) {
	pod = entity(kube.KindPod, name+"-0")
	rs := entity(kube.KindReplicaSet, name+"-rs")
	deploy = entity(kube.KindDeployment, name)
	r.relate(pod, knowledge.OwnedBy, rs)
	r.relate(rs, knowledge.OwnedBy, deploy)
	return pod, deploy
}

func TestManagerGroupsReplicasUnderTheirWorkload(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	other := entity(kube.KindPod, "web-1")
	r.relate(other, knowledge.OwnedBy, entity(kube.KindReplicaSet, "web-rs"))

	r.raise(at(0), sig(pod, constant.ReasonCrashLoopBackOff, signal.Warning),
		sig(other, constant.ReasonCrashLoopBackOff, signal.Warning))

	got := r.only()
	if got.Root != deploy || len(got.Members) != 2 {
		t.Fatalf("want one deployment problem, got %+v", got)
	}
}

func TestManagerContainerSignalBelongsToPodWorkload(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	container := entity(kube.KindContainer, "web-0/app")
	r.relate(container, knowledge.PartOf, pod)

	r.raise(at(0), sig(container, constant.ReasonOOMKilled, signal.Warning))
	if got := r.only(); got.Root != deploy {
		t.Fatalf("root = %v, want %v", got.Root, deploy)
	}
}

func TestManagerAttributesSymptomToCauseRoot(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	node := entity(kube.KindNode, "n1")
	h := r.cause(pod, node, "node ran out of memory")

	r.raise(at(0), sig(pod, constant.ReasonCrashLoopBackOff, signal.Warning))
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
	r.sigs[pod] = []signal.Signal{
		sig(pod, constant.ReasonCrashLoopBackOff, signal.Warning),
	}

	r.raise(at(0), sig(deploy, "Unavailable", signal.Critical))
	got := r.only()
	if got.Root != deploy || got.Cause != nil {
		t.Fatalf("want own root without cause, got %+v", got)
	}
}

func TestManagerFollowsCauseChainToDeepestRoot(t *testing.T) {
	cases := map[string]func(r *rig, deploy, pod knowledge.EntityID){
		"root signals": func(r *rig, deploy, _ knowledge.EntityID) {
			svc := entity(kube.KindService, "web")
			h := r.cause(svc, deploy, "svc")
			h.RootSignals = []signal.Signal{
				sig(deploy, "Stuck", signal.Warning),
			}
			r.rule.causes[svc] = h
		},
		"owned pod signal": func(r *rig, deploy, pod knowledge.EntityID) {
			r.cause(entity(kube.KindService, "web"), deploy, "svc")
			r.sigs[pod] = []signal.Signal{sig(pod, "Stuck", signal.Warning)}
		},
		"owned container signal": func(r *rig, deploy, pod knowledge.EntityID) {
			r.cause(entity(kube.KindService, "web"), deploy, "svc")
			c := entity(kube.KindContainer, "web-0/app")
			r.relate(c, knowledge.PartOf, pod)
			r.sigs[c] = []signal.Signal{sig(c, "Stuck", signal.Warning)}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, Config{})
			pod, deploy := workload(r, "web")
			node := entity(kube.KindNode, "n1")
			setup(r, deploy, pod)
			r.cause(deploy, node, "node down")
			r.cause(pod, node, "node down")
			r.cause(entity(kube.KindContainer, "web-0/app"),
				node, "node down")

			svc := entity(kube.KindService, "web")
			r.raise(at(0), sig(svc, "NoEndpoints", signal.Critical))
			if got := r.only(); got.Root != node {
				t.Fatalf("root = %v, want %v", got.Root, node)
			}
		})
	}
}

func TestManagerStopsChainAtChangeCause(t *testing.T) {
	r := newRig(t, Config{})
	deploy := entity(kube.KindDeployment, "web")
	svc := entity(kube.KindService, "web")
	h := r.cause(svc, deploy, "new image rolled out")
	h.Change = &knowledge.Change{}
	h.RootSignals = []signal.Signal{sig(deploy, "Stuck", signal.Warning)}
	r.rule.causes[svc] = h
	r.cause(deploy, entity(kube.KindNode, "n1"), "unreachable")

	r.raise(at(0), sig(svc, "NoEndpoints", signal.Critical))
	if got := r.only(); got.Root != deploy || got.Cause.Change == nil {
		t.Fatalf("want change cause at deployment, got %+v", got)
	}
}

func TestManagerReattachesSignalWhenCauseIsRevised(t *testing.T) {
	r := newRig(t, Config{})
	pod := podSig("web")
	r.raise(at(0), pod)
	node := entity(kube.KindNode, "n1")
	r.cause(pod.Entity, node, "node down")
	r.apply(at(1), signal.Changed, pod)

	records := r.m.Export()
	if len(records) != 2 {
		t.Fatalf("want old and new problem, got %d", len(records))
	}
	for _, rec := range records {
		p := r.m.problems[rec.ID]
		switch rec.Root {
		case node:
			if len(p.Members) != 1 || !hasNote(p, "cause revised") {
				t.Fatalf("node problem wrong: %+v", p)
			}
		default:
			if len(p.Members) != 0 || !hasNote(p, "recovered") {
				t.Fatalf("old problem should lose member: %+v", p)
			}
		}
	}
}

func hasNote(p *Problem, prefix string) bool {
	for _, e := range p.Timeline {
		if len(e.Text) >= len(prefix) && e.Text[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func TestManagerClearingUnknownSignalIsIgnored(t *testing.T) {
	r := newRig(t, Config{})
	r.clear(at(0), podSig("ghost"))
	if len(r.m.Export()) != 0 {
		t.Fatal("clearing an unknown signal must not create a problem")
	}
}

func TestManagerImpactListsWorkloadServiceAndIngress(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	slice := entity(kube.KindEndpointSlice, "web-x")
	svc := entity(kube.KindService, "web")
	ingress := entity(kube.KindIngress, "web")
	r.relate(slice, knowledge.RoutesTo, pod)
	r.relate(slice, knowledge.Backs, svc)
	r.relate(ingress, knowledge.RoutesTo, svc)

	r.raise(at(0), sig(pod, constant.ReasonCrashLoopBackOff,
		signal.Critical))
	got := r.only()
	if got.Root != deploy || !userFacing(got.Impact) ||
		got.Tier != Page {
		t.Fatalf("want user-facing page, got %+v", got)
	}
	if impactSize(&got) != 2 {
		t.Fatalf("impact size = %d, want 2 (%v)",
			impactSize(&got), got.Impact)
	}
}

func TestManagerOwnedPodsOfPodIsItself(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "solo")
	got := ownedPods(r.model, pod)
	if len(got) != 1 || got[0] != pod {
		t.Fatalf("ownedPods = %v", got)
	}
}

func TestManagerDescribeIncludesNamespaceWhenSet(t *testing.T) {
	ns := knowledge.NewEntityID(kube.KindNode, "", "n1")
	if got := describe(ns); got != "node n1" {
		t.Fatalf("describe = %q", got)
	}
	if got := describe(entity(kube.KindPod, "p")); got != "pod shop/p" {
		t.Fatalf("describe = %q", got)
	}
}
