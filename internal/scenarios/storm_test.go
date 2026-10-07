package scenarios

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// Storm sizes from docs/production-goals.md: 1,000 failing pods in 500
// workloads.
const (
	stormWorkloads = 500
	stormReplicas  = 2
)

var stormStart = time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)

// stormResult is how loud one storm was.
type stormResult struct {
	name     string
	pods     int
	messages int
	// peak is the most messages inside any two minutes.
	peak int
}

// storm is a generated storm: its log and the size of the failure.
type storm struct {
	name string
	pods int
	log  replay.Log
}

// runStorms replays the storms side by side, each as a parallel subtest.
func runStorms(t *testing.T) []stormResult {
	t.Helper()
	storms := []storm{sharedNodeStorm(), sharedRegistryStorm()}
	out := make([]stormResult, len(storms))
	t.Run("each", func(t *testing.T) {
		for i, s := range storms {
			t.Run(s.name, func(t *testing.T) {
				t.Parallel()
				result := replayLog(t, s.log, replay.Options{})
				out[i] = stormResult{
					name: s.name, pods: s.pods,
					messages: len(result.Messages),
					peak: scorecard.PeakInWindow(result.Times,
						scorecard.GoalStormWindow),
				}
			})
		}
	})
	return out
}

// sharedNodeStorm is 1,000 pods of 500 workloads on one node that stops
// reporting. The node lifecycle controller marks every pod not ready.
func sharedNodeStorm() storm {
	c := newCluster(stormStart, "")
	node := c.node("storm-node", "zone-a")
	c.list(node, c.node("spare-node", "zone-a"))
	workloads := stormFleet(c, "registry.example.com/fleet",
		func(*workload, int) string { return "storm-node" })
	c.after(time.Minute)
	lost := last(c, node)
	setNodeCondition(lost, corev1.NodeReady, corev1.ConditionUnknown,
		"NodeStatusUnknown", "Kubelet stopped posting node status.", c.now)
	c.update(lost)
	c.after(40 * time.Second)
	for _, w := range workloads {
		for i := range stormReplicas {
			c.update(w.pod(i, "storm-node", notReady))
		}
		w.setReady(0)
		c.update(w.objects())
	}
	return storm{name: "shared node", pods: stormPods(), log: c.log()}
}

// sharedRegistryStorm is 1,000 pods of 500 workloads whose node pool is
// replaced: every pod is recreated on a fresh node without an image cache,
// and every pull fails because the pull credential for the one private
// registry has expired.
func sharedRegistryStorm() storm {
	c := newCluster(stormStart, "")
	for i := range 10 {
		c.list(c.node(fmt.Sprintf("old-%d", i), "zone-a"))
	}
	spread := func(prefix string) func(*workload, int) string {
		return func(_ *workload, i int) string {
			return fmt.Sprintf("%s-%d", prefix, i%10)
		}
	}
	workloads := stormFleet(c, "registry.corp.example/base", spread("old"))
	c.after(time.Minute)
	for i := range 10 {
		c.create(c.node(fmt.Sprintf("new-%d", i), "zone-a"))
	}
	place := spread("new")
	for i, w := range workloads {
		image := w.deployment.Spec.Template.Spec.Containers[0].Image
		message := "Back-off pulling image \"" + image + "\": " +
			"ErrImagePull: failed to pull and unpack image \"" + image +
			"\": failed to resolve reference: pulling from host " +
			"registry.corp.example failed with status code [manifests " +
			"v1]: 401 Unauthorized"
		for r := range stormReplicas {
			c.remove(last(c, w.pod(r, "")))
			c.create(w.pod(r+stormReplicas, place(w, i*stormReplicas+r),
				startedNow, waiting("ImagePullBackOff", message)))
		}
		w.setReady(0)
		c.update(w.objects())
		if i%50 == 49 {
			c.after(3 * time.Second)
		}
	}
	return storm{name: "shared registry", pods: stormPods(), log: c.log()}
}

func stormPods() int { return stormWorkloads * stormReplicas }

// stormFleet lists 500 healthy two-replica Deployments, placing each
// replica with place.
func stormFleet(
	c *cluster, repository string, place func(*workload, int) string,
) []*workload {
	workloads := make([]*workload, 0, stormWorkloads)
	for i := range stormWorkloads {
		w := c.deployment(fmt.Sprintf("team-%d", i%20),
			fmt.Sprintf("svc-%d", i),
			fmt.Sprintf("%s/svc-%d:v1", repository, i), stormReplicas)
		objects := []runtime.Object{w.deployment, w.replicaSet}
		for r := range stormReplicas {
			objects = append(objects, w.pod(r, place(w, i*stormReplicas+r)))
		}
		c.list(objects...)
		workloads = append(workloads, w)
	}
	return workloads
}
