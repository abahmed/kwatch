package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const envPath = "spec.template.spec.containers[0].env"

// replicaFixture is a Deployment of three replicas whose spec changed:
// replicas 0 and 1 run on n1 and replica 2 on n2. failing says which
// replicas crash; the others are ready.
func replicaFixture(t *testing.T, failing ...int) *fixture {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("shop", "api", 3, nodes[0])
	f.relate(pods[2], inventory.RunsOn, nodes[1])
	f.ready(pods...)
	for _, i := range failing {
		f.crash(pods[i])
	}
	f.change(deployment("shop", "api"), 1, envPath)
	return f
}

func TestScoreReplicas(t *testing.T) {
	dep := deployment("shop", "api")
	t.Run("failures confined to one node", func(t *testing.T) {
		f := replicaFixture(t, 0, 1)
		o := scoreOf(t, f, dep, scoreReplicas)
		requireWeight(t, o, -NodeLocalPenalty)
		if o.code != rootcause.ProofReplicasNodeLocal {
			t.Fatalf("code = %q", o.code)
		}
	})
	t.Run("one failing pod is just a pod", func(t *testing.T) {
		requireWeight(t, scoreOf(t, replicaFixture(t, 0), dep,
			scoreReplicas), 0)
	})
	t.Run("healthy replica on the same node", func(t *testing.T) {
		f := newFixture(t)
		nodes := f.nodes("zone-a", "n1", "n2")
		pods := f.workload("shop", "api", 4, nodes[0])
		f.relate(pods[3], inventory.RunsOn, nodes[1])
		f.ready(pods...)
		f.crash(pods[0], pods[1])
		f.change(dep, 1, envPath)
		// pods[2] is healthy on n1 too: the node is not the difference.
		requireWeight(t, scoreOf(t, f, dep, scoreReplicas), 0)
	})
	t.Run("every replica fails on several nodes", func(t *testing.T) {
		o := scoreOf(t, replicaFixture(t, 0, 1, 2), dep, scoreReplicas)
		requireWeight(t, o, EverywhereWeight)
		if o.code != rootcause.ProofReplicasEverywhere {
			t.Fatalf("code = %q", o.code)
		}
	})
	t.Run("a node is not judged", func(t *testing.T) {
		f := replicaFixture(t, 0, 1)
		node := inventory.CoreID(kube.KindNode, "", "n1")
		f.fail(node, "NotReady", failingH, 1, "")
		requireWeight(t, scoreOf(t, f, node, scoreReplicas), 0)
	})
}

// peersFixture runs two failing replicas of api on n1 beside peers pods
// of other workloads; failing peers of them crash.
func peersFixture(t *testing.T, peers, failingPeers int) *fixture {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("shop", "api", 2, nodes[0])
	f.ready(pods...)
	f.crash(pods...)
	f.change(deployment("shop", "api"), 1, envPath)
	for i := range peers {
		peer := f.workload("shop", "peer"+string(rune('a'+i)), 1, nodes[0])
		f.ready(peer...)
		if i < failingPeers {
			f.crash(peer...)
		}
	}
	return f
}

func TestScoreNodePeers(t *testing.T) {
	dep := deployment("shop", "api")
	t.Run("healthy neighbours clear the node", func(t *testing.T) {
		o := scoreOf(t, peersFixture(t, NodePeersMin, 0), dep,
			scoreNodePeers)
		requireWeight(t, o, NodePeersWeight)
		if o.code != rootcause.ProofNodePeersHealthy ||
			o.count != NodePeersMin {
			t.Fatalf("outcome = %+v", o)
		}
	})
	t.Run("too few neighbours", func(t *testing.T) {
		requireWeight(t, scoreOf(t, peersFixture(t, NodePeersMin-1, 0),
			dep, scoreNodePeers), 0)
	})
	t.Run("failing neighbours keep the node suspect", func(t *testing.T) {
		requireWeight(t, scoreOf(t, peersFixture(t, NodePeersMin+1, 1),
			dep, scoreNodePeers), 0)
	})
	t.Run("a node is never raised", func(t *testing.T) {
		f := peersFixture(t, NodePeersMin, 0)
		node := inventory.CoreID(kube.KindNode, "", "n1")
		f.fail(node, "NotReady", failingH, 1, "")
		requireWeight(t, scoreOf(t, f, node, scoreNodePeers), 0)
	})
}

func TestScoreConfigElsewhere(t *testing.T) {
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "api-config")
	build := func(t *testing.T, twinHealthy bool) *fixture {
		f := newFixture(t)
		f.add(cm)
		pods := f.workload("shop", "api", 2)
		twin := f.workload("shop", "batch", 1)
		f.ready(twin...)
		f.crash(pods...)
		if !twinHealthy {
			f.crash(twin...)
		}
		for _, pod := range append(pods, twin...) {
			f.relate(pod, inventory.References, cm)
		}
		f.change(cm, 1, "data.app.yaml")
		return f
	}
	t.Run("healthy user lowers the config", func(t *testing.T) {
		o := scoreOf(t, build(t, true), cm, scoreConfigElsewhere)
		requireWeight(t, o, -ConfigElsewhereWeight)
		if o.code != rootcause.ProofConfigElsewhere {
			t.Fatalf("code = %q", o.code)
		}
	})
	t.Run("failing user says nothing", func(t *testing.T) {
		requireWeight(t, scoreOf(t, build(t, false), cm,
			scoreConfigElsewhere), 0)
	})
}

func TestScorePreviousHealthy(t *testing.T) {
	dep := deployment("shop", "api")
	build := func(t *testing.T, readyFor int) *fixture {
		f := newFixture(t)
		f.now = t0.Add(40 * time.Minute)
		nodes := f.nodes("zone-a", "n1", "n2")
		pods := f.workload("shop", "api", 2, nodes...)
		f.crash(pods...)
		old := inventory.CoreID(kube.KindReplicaSet, "shop", "api-0")
		f.add(old)
		f.relate(old, inventory.OwnedBy, dep)
		f.revision(inventory.CoreID(kube.KindReplicaSet, "shop", "api-1"), "2")
		f.revision(old, "1")
		prev := inventory.CoreID(kube.KindPod, "shop", "api-0-0")
		f.add(prev)
		f.relate(prev, inventory.OwnedBy, old)
		f.readyFor(prev, readyFor)
		f.change(dep, 20, envPath)
		return f
	}
	t.Run("ran healthy long enough", func(t *testing.T) {
		o := scoreOf(t, build(t, 30), dep, scorePreviousHealthy)
		requireWeight(t, o, PreviousHealthyWeight)
		if o.code != rootcause.ProofPreviousHealthy || o.count < 10 {
			t.Fatalf("outcome = %+v", o)
		}
	})
	t.Run("ready only just before the change", func(t *testing.T) {
		requireWeight(t, scoreOf(t, build(t, 5), dep,
			scorePreviousHealthy), 0)
	})
}
