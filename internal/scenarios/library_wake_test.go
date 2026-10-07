package scenarios

import (
	"strconv"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

// wakeScenarios are a cluster scaled to zero at night and scaled up
// again in the morning, and the cases that must not look like it.
func wakeScenarios() []scenario {
	return []scenario{
		wakeUpBlipsHeld(), wakeUpCrashLoop(), smallScaleUpNodeLost(),
		plannedScaleDownQuiet(),
	}
}

// wakeApps is how many workloads the morning scale-up starts.
const wakeApps = 6

// wakeReadyDelay is how long the slowest dependency takes: pods fail
// their readiness probe for this long after they start.
const wakeReadyDelay = 9 * time.Minute

func wakeName(i int) string { return "wake-app" + strconv.Itoa(i) }

// wakeFleet lists the nodes and wakeApps Deployments at zero replicas, as
// the night left them, then lets half an hour pass.
func wakeFleet(c *cluster) []*workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	var fleet []*workload
	for i := range wakeApps {
		w := c.deployment("shop", wakeName(i),
			"registry.example.com/app:1.0", 0)
		c.list(w.objects())
		fleet = append(fleet, w)
	}
	c.after(30 * time.Minute)
	return fleet
}

// wakeStart scales w to two replicas; both pods start not ready.
func wakeStart(c *cluster, w *workload) []time.Time {
	two := int32(2)
	w.deployment.Spec.Replicas = &two
	w.replicaSet.Spec.Replicas = &two
	w.setReady(0)
	c.update(w.objects())
	created := make([]time.Time, 0, 2)
	for j, node := range []string{"n1", "n2"} {
		c.create(w.pod(j, node, startedNow, notReady))
		created = append(created, c.now)
	}
	return created
}

// wakeProbeFailures reports the readiness probe failing on both pods.
func wakeProbeFailures(c *cluster, w *workload, created []time.Time) {
	for j, node := range []string{"n1", "n2"} {
		pod := w.pod(j, node, createdAt(created[j]), notReady)
		c.warn(c.warningEvent(pod, "Pod", "Unhealthy",
			"Readiness probe failed: HTTP probe failed with statuscode: 503",
			"kubelet", 3))
	}
}

// wakeReady makes both pods of w ready.
func wakeReady(c *cluster, w *workload, created []time.Time) {
	for j, node := range []string{"n1", "n2"} {
		c.update(w.pod(j, node, createdAt(created[j])))
	}
	w.setReady(2)
	c.update(w.objects())
}

// wakeUp starts the fleet, fifteen seconds apart. crash, when it is not
// negative, is the index of the workload whose pods crash-loop instead
// of recovering. The probes fail for wakeReadyDelay after the first
// start; the others recover then.
func wakeUp(c *cluster, fleet []*workload, crash int) {
	first := c.now
	created := make([][]time.Time, len(fleet))
	for i, w := range fleet {
		created[i] = wakeStart(c, w)
		c.after(15 * time.Second)
	}
	c.after(2 * time.Minute)
	for i, w := range fleet {
		wakeProbeFailures(c, w, created[i])
	}
	c.after(first.Add(wakeReadyDelay).Sub(c.now))
	for i, w := range fleet {
		if i != crash {
			wakeReady(c, w, created[i])
		}
	}
	if crash >= 0 {
		for restarts := int32(2); restarts <= 6; restarts += 2 {
			for j, node := range []string{"n1", "n2"} {
				w := fleet[crash]
				c.update(w.pod(j, node, createdAt(created[crash][j]),
					crashLoop(1, "Error", "panic: cannot reach the "+
						"database", restarts)))
			}
			c.after(time.Minute)
		}
	}
}

// wakeUpBlipsHeld: the morning scale-up starts six workloads and every
// pod fails its readiness probe for nine minutes while a dependency
// comes up. All recover inside the wake-up. Nobody is interrupted; the
// next digest carries one line that says so.
func wakeUpBlipsHeld() scenario {
	return scenario{
		expect: expectation{
			Name: "wake-up-blips-held",
			Description: "Six Deployments scale from zero to two " +
				"replicas within two minutes; their pods fail the " +
				"readiness probe for nine minutes, then all pass.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) { wakeUp(c, wakeFleet(c), -1) },
	}
}

// wakeUpCrashLoop: the same morning, but one workload's pods crash-loop
// on a dependency that never comes up. That is a failure, not a blip.
func wakeUpCrashLoop() scenario {
	return scenario{
		expect: expectation{
			Name: "wake-up-crash-loop",
			Description: "Six Deployments scale from zero while a " +
				"dependency is down; one workload's pods crash-loop and " +
				"the rest recover.",
			Root: "deployment/shop/wake-app2", Tier: "notify",
			MaxMessages: 3, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) { wakeUp(c, wakeFleet(c), 2) },
	}
}

// smallScaleUpNodeLost: four workloads start from zero, fewer than a
// wake-up, and ten minutes later one node of the cluster is lost. The
// pods on it go not ready: the node pages, exactly as without the
// scale-up.
func smallScaleUpNodeLost() scenario {
	return scenario{
		expect: expectation{
			Name: "small-scale-up-node-lost",
			Description: "Four Deployments scale up from zero (no " +
				"wake-up); ten minutes later a node stops reporting " +
				"and the pods of eight workloads on it go not ready.",
			Root: "node//n1", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"zone//zone-a"},
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			names := make([]string, 0, 8)
			for i := range 8 {
				names = append(names, "svc-"+strconv.Itoa(i))
			}
			fleet := nodeFleet(c, "apps", names, []string{"n1", "n2"})
			c.after(time.Minute)
			for i := range wakeApps - 2 {
				w := c.deployment("shop", wakeName(i),
					"registry.example.com/app:1.0", 0)
				c.list(w.objects())
				created := wakeStart(c, w)
				c.after(3 * time.Minute)
				wakeReady(c, w, created)
			}
			c.after(10 * time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			for _, w := range fleet {
				c.update(w.pod(0, "n1", notReady))
				w.setReady(1)
				c.update(w.deployment, w.replicaSet)
			}
			c.after(5 * time.Minute)
		},
	}
}

// plannedScaleDownQuiet: the evening scale-down sets five routed
// workloads to zero replicas and drains one node, then removes it. The
// Ingresses still route to the Services, but nobody is surprised.
func plannedScaleDownQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "planned-scale-down-quiet",
			Description: "Five Deployments with Ingresses are scaled to " +
				"zero within a minute and a node is cordoned, " +
				"drained and removed.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			n2 := c.node("n2", "zone-a")
			c.list(c.node("n1", "zone-a"), n2)
			var fleet []*workload
			for i := range 5 {
				name := wakeName(i)
				w := c.deployment("shop", name,
					"registry.example.com/app:1.0", 2)
				c.list(w.deployment, w.replicaSet, w.pod(0, "n1"), w.pod(1, "n2"))
				c.list(clusterService(c, "shop", name, 8080))
				c.list(scaleDownIngress(c, name))
				fleet = append(fleet, w)
			}
			c.after(30 * time.Minute)
			for _, w := range fleet {
				setToZero(c, w)
				c.after(15 * time.Second)
			}
			cordoned := last(c, n2)
			cordoned.Spec.Unschedulable = true
			c.update(cordoned)
			c.after(3 * time.Minute)
			c.remove(n2)
		},
	}
}

// setToZero sets the replicas of w to zero and removes its pods.
func setToZero(c *cluster, w *workload) {
	zero := int32(0)
	w.deployment.Spec.Replicas = &zero
	w.replicaSet.Spec.Replicas = &zero
	w.setReady(0)
	c.update(w.objects())
	c.remove(w.pod(0, "n1"), w.pod(1, "n2"))
}

// scaleDownIngress routes traffic to the Service of the named workload.
func scaleDownIngress(c *cluster, backend string) *networkingv1.Ingress {
	ing := trafficIngress(c, backend, "")
	ing.Name = c.n(backend)
	ing.UID += types.UID(backend)
	return ing
}
