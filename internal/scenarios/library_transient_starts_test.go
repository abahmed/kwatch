package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// transientStartScenarios are start-up hiccups that pass by themselves:
// a sandbox the kubelet's retry sets up, a young node's address pool
// warming up, a scale-up's new replica that is not ready yet.
func transientStartScenarios() []scenario {
	return []scenario{sandboxFailsOnceThenStarts(),
		youngNodeAddressWarmUp(), scaleUpBackendStarting()}
}

// sandboxFailsOnceThenStarts: a rollout replaces six pods on old nodes.
// Each new pod's first sandbox attempt fails and the kubelet's retry a
// second later works. Nothing needs a person.
func sandboxFailsOnceThenStarts() scenario {
	return scenario{
		expect: expectation{
			Name: "sandbox-first-attempt-retried",
			Description: "Six new pods each fail their first sandbox " +
				"attempt and start a second later.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:4", 6)
			c.list(w.objects())
			c.after(10 * time.Minute)
			born := c.now
			for i := range 6 {
				node := []string{"n1", "n2"}[i%2]
				pod := w.pod(i, node, createdAt(born), creating)
				c.create(pod)
				ev := c.warningEvent(pod, "Pod", "FailedCreatePodSandBox",
					sandboxRefusedMessage, "kubelet", 1)
				ev.InvolvedObject.UID = pod.UID
				c.warn(ev)
			}
			c.after(time.Second)
			for i := range 6 {
				node := []string{"n1", "n2"}[i%2]
				c.update(w.pod(i, node, startedNow, createdAt(born)))
			}
		},
	}
}

// youngNodeAddressWarmUp: a node joins and its address manager has no
// free address yet, so the first pods bound to it fail with "failed to
// assign an IP address" for a minute and a half, then all start. It is
// the node's warm-up, not an exhausted pool.
func youngNodeAddressWarmUp() scenario {
	return scenario{
		expect: expectation{
			Name: "young-node-address-warm-up",
			Description: "Pods on a node that just joined fail address " +
				"assignment for 90 seconds, then start.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:7", 4)
			c.list(w.objects())
			c.after(10 * time.Minute)
			n2 := c.node("n2", "zone-a")
			n2.CreationTimestamp = metav1.NewTime(c.now)
			c.create(n2)
			born := c.now
			pods := make([]*corev1.Pod, 4)
			for i := range pods {
				pods[i] = w.pod(i, "n2", createdAt(born), creating)
				c.create(pods[i])
			}
			for round := int32(1); round <= 3; round++ {
				c.after(30 * time.Second)
				for _, pod := range pods {
					ev := c.warningEvent(pod, "Pod",
						"FailedCreatePodSandBox", ipExhaustedMessage,
						"kubelet", round)
					ev.InvolvedObject.UID = pod.UID
					c.warn(ev)
				}
			}
			c.after(10 * time.Second)
			for i := range pods {
				c.update(w.pod(i, "n2", startedNow, createdAt(born)))
			}
		},
	}
}

// scaleUpBackendStarting: a Service's Deployment is scaled from two to
// three replicas; the new pod takes 20 seconds to become ready, so the
// Service has 2 of 3 ready backends meanwhile. That is a scale-up.
func scaleUpBackendStarting() scenario {
	return scenario{
		expect: expectation{
			Name: "scale-up-backend-starting",
			Description: "A Service has 2 of 3 ready backends for 20 " +
				"seconds while a scale-up's new pod starts.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:9", 2)
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(w.objects())
			c.list(pods[0], pods[1], clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			c.after(10 * time.Minute)
			setReplicas(w, 3)
			w.setReady(2)
			c.update(w.objects())
			born := c.now
			fresh := w.pod(2, "n1", createdAt(born), creating)
			c.create(fresh)
			slice := trafficSlice(c, "web", pods[0], pods[1], fresh)
			slice.Endpoints[2].Conditions.Ready = boolPtr(false)
			c.update(slice)
			c.after(20 * time.Second)
			started := w.pod(2, "n1", startedNow, createdAt(born))
			c.update(started)
			w.setReady(3)
			c.update(w.objects())
			c.update(trafficSlice(c, "web", pods[0], pods[1], started))
		},
	}
}
