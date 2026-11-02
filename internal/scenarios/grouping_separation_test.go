package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/replay"
)

// replayBuilt records build into a log and replays it. The stories are
// local to these tests: they carry no label and no golden file.
func replayBuilt(
	t *testing.T, tail time.Duration, build func(c *cluster),
) replay.Result {
	t.Helper()
	s := scenario{build: build}
	log := s.generate(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), "")
	return replayLog(t, log, replay.Options{Tail: tail})
}

// announcedRoots maps the root of every incident tracked at the end of
// the replay to the incident. A burst of new incidents is told in one
// summary message, so the incidents, not the messages, show the grouping.
func announcedRoots(result replay.Result) map[string]incident.Incident {
	out := map[string]incident.Incident{}
	for _, p := range result.Incidents {
		out[p.Root.String()] = p
	}
	return out
}

// memberNames lists the entities of an incident's members.
func memberNames(p incident.Incident) []string {
	var out []string
	for key := range p.Members {
		out = append(out, key.Entity.String())
	}
	return out
}

// ownsNothingOf reports whether none of the members of p belongs to the
// workload app, a Deployment name such as "pay".
func ownsNothingOf(p incident.Incident, app string) bool {
	for _, name := range memberNames(p) {
		if strings.Contains(name, "/"+app) {
			return false
		}
	}
	return true
}

// crashTwoAtOnce crash-loops two workloads in the same second, each with
// its own error and on its own node.
func crashTwoAtOnce(namespaces [2]string) func(c *cluster) {
	return func(c *cluster) {
		c.list(c.node("n0", "zone-a"), c.node("n1", "zone-a"))
		var fleet []*workload
		for i, node := range []string{"n0", "n1"} {
			w := c.deployment(namespaces[i], []string{"orders", "search"}[i],
				"registry.example.com/app:1.0", 1)
			c.list(w.deployment, w.replicaSet, w.pod(0, node))
			fleet = append(fleet, w)
		}
		c.after(2 * time.Minute)
		errs := []string{"fatal: ledger lost its own dependency",
			"fatal: geocoder lost its own dependency"}
		for restarts := int32(1); restarts <= 5; restarts++ {
			for i, w := range fleet {
				c.update(w.pod(0, []string{"n0", "n1"}[i], crashLoop(1,
					"Error", errs[i], restarts)))
				w.setReady(0)
				c.update(w.objects())
			}
			c.after(20 * time.Second)
		}
	}
}

// Two unrelated failures at the same time are two incidents: neither
// explains the other, so neither takes the other as a member.
func TestSimultaneousUnrelatedFailuresStaySeparate(t *testing.T) {
	cases := map[string][2]string{
		"same namespace":       {"shop", "shop"},
		"different namespaces": {"shop", "billing"},
	}
	for name, ns := range cases {
		t.Run(name, func(t *testing.T) {
			result := replayBuilt(t, 10*time.Minute, crashTwoAtOnce(ns))

			roots := announcedRoots(result)
			orders := roots["deployment/"+ns[0]+"/orders"]
			search := roots["deployment/"+ns[1]+"/search"]
			require.Len(t, roots, 2, pageTexts(result))
			assert.True(t, ownsNothingOf(orders, "search"), pageTexts(result))
			assert.True(t, ownsNothingOf(search, "orders"), pageTexts(result))
		})
	}
}

// buildOutageWithOwnCause crash-loops eight of fifteen Deployments in
// shop at once, each with its own error: a namespace outage. Two minutes
// into it app8, in the same namespace, starts crashing after its own bad
// release, a cause of its own.
func buildOutageWithOwnCause(c *cluster) {
	fleet, nodes := outageFleet(c, 15)
	c.after(2 * time.Minute)
	for restarts := int32(1); restarts <= 5; restarts++ {
		for i := range 8 {
			outageCrash(c, fleet[i], nodes[i], i, restarts)
			c.after(time.Second)
		}
		if restarts == 2 {
			releaseCrash(c, fleet[8], nodes[8])
		}
		c.after(20 * time.Second)
	}
}

// releaseCrash rolls w out to an image that panics on start.
func releaseCrash(c *cluster, w *workload, node string) {
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/app8:2.0"
	})
	c.update(w.deployment)
	c.create(rs)
	for restarts := int32(1); restarts <= 3; restarts++ {
		c.update(w.pod(0, node, startedNow, crashLoop(2, "Error",
			"panic: assignment to entry in nil map", restarts)))
		c.after(20 * time.Second)
	}
}

// A failure with a cause of its own that lands while a namespace outage
// is forming is not folded into it: the namespace message does not
// speak for it.
func TestOwnCauseDuringNamespaceOutageIsNotFolded(t *testing.T) {
	result := replayBuilt(t, 15*time.Minute, buildOutageWithOwnCause)

	roots := announcedRoots(result)
	own, ok := roots["deployment/shop/app8"]
	require.True(t, ok, pageTexts(result))
	assert.False(t, own.Standalone(), "app8 has its own cause")
	outage := false
	for i, m := range result.Messages {
		text := notification.Text(m)
		if strings.Contains(text, "app0") {
			outage = true
			assert.NotContains(t, text, "app8", "message %d:\n%s", i,
				pageTexts(result))
		}
	}
	assert.True(t, outage, "the namespace outage was told")
}

// buildNodeLostThenOther loses node n2 and, thirty seconds later, while
// that outage is still settling, starts another failure on other nodes.
func buildNodeLostThenOther(other func(c *cluster, fleet []*workload)) func(
	c *cluster,
) {
	return func(c *cluster) {
		n2 := c.node("n2", "zone-a")
		c.list(c.node("n1", "zone-a"), n2, c.node("n3", "zone-b"))
		nodes := []string{"n1", "n2", "n3"}
		fleet := nodeFleet(c, "shop", []string{"cart", "pay"}, nodes)
		c.after(2 * time.Minute)
		nodeLose(c, n2)
		c.after(30 * time.Second)
		other(c, fleet)
		for range 8 {
			c.after(time.Minute)
		}
	}
}

// An application release that breaks while a node is being lost has its
// own cause: the node outage does not take it in.
func TestOwnCauseDuringNodeOutageSettleIsNotMerged(t *testing.T) {
	result := replayBuilt(t, 10*time.Minute, buildNodeLostThenOther(
		func(c *cluster, fleet []*workload) {
			pay := fleet[1]
			rs := pay.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/pay:2.0"
			})
			c.update(pay.deployment)
			c.create(rs)
			for restarts := int32(1); restarts <= 4; restarts++ {
				c.update(pay.pod(0, "n1", startedNow, crashLoop(2, "Error",
					"panic: assignment to entry in nil map", restarts)),
					pay.pod(2, "n3", startedNow, crashLoop(2, "Error",
						"panic: assignment to entry in nil map", restarts)))
				c.after(30 * time.Second)
			}
		}))

	roots := announcedRoots(result)
	node, ok := roots["node//n2"]
	require.True(t, ok, pageTexts(result))
	assert.Empty(t, node.Merged, pageTexts(result))
	assert.True(t, ownsNothingOf(node, "pay"), memberNames(node))
	own, ok := roots["deployment/shop/pay"]
	require.True(t, ok, pageTexts(result))
	assert.NotContains(t, memberNames(own), "node//n2")
}

// A second node with a different problem (disk pressure) is not part of
// a node that stopped reporting.
func TestOtherNodeProblemDuringNodeOutageIsNotMerged(t *testing.T) {
	result := replayBuilt(t, 10*time.Minute, buildNodeLostThenOther(
		func(c *cluster, _ []*workload) {
			n3 := last(c, c.node("n3", "zone-b"))
			c.update(rcaDiskPressure(c, n3, true, c.now))
		}))

	n3 := inventory.CoreID(kube.KindNode, "", "n3")
	for _, d := range result.Decisions {
		assert.NotContains(t, d.Incident.Merged, n3, pageTexts(result))
	}
	roots := announcedRoots(result)
	node, ok := roots["node//n2"]
	require.True(t, ok, pageTexts(result))
	assert.NotContains(t, memberNames(node), "node//n3")
}
