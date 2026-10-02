package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// workloadConfigScenarios are workloads broken by their own settings:
// a probe aimed at the wrong port, a startup probe shorter than the
// application's start. The memory limit case is oom-limit-too-low.
func workloadConfigScenarios() []scenario {
	return []scenario{probePortMismatch(), startupBudgetTooShort()}
}

// probePortMismatch: a release moves the readiness probe to port 8081,
// which the container does not serve; the new pods never become ready.
// The Deployment is the root, through its probe, not the nodes.
func probePortMismatch() scenario {
	return scenario{
		expect: expectation{
			Name: "probe-port-mismatch",
			Description: "A release points the readiness probe at a " +
				"port the container does not declare; new pods stay " +
				"unready.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:5", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				configProbe(spec, 8080)
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				configProbe(spec, 8081)
			})
			c.update(w.deployment)
			c.create(rs)
			c.create(w.pod(0, "n1", startedNow, notReady),
				w.pod(1, "n2", startedNow, notReady))
			for range 5 {
				c.after(time.Minute)
				for i, node := range []string{"n1", "n2"} {
					pod := last(c, w.pod(i, node))
					ev := c.warningEvent(pod, "Pod", "Unhealthy",
						"Readiness probe failed: Get \"http://10.244.0.9:"+
							"8081/healthz\": dial tcp 10.244.0.9:8081: "+
							"connect: connection refused", "kubelet", 5)
					// The kubelet names the probed container.
					ev.InvolvedObject.FieldPath = "spec.containers{app}"
					c.warn(ev)
				}
			}
		},
	}
}

// startupBudgetTooShort: the api scales out; its ready replicas took 75
// seconds to start, but the startup probe gives up after 30, so every
// new replica is killed before it is up. The Deployment is the root.
func startupBudgetTooShort() scenario {
	return scenario{
		expect: expectation{
			Name: "startup-budget-too-short",
			Description: "New replicas are killed by a startup probe " +
				"that allows 30s, while ready replicas needed 75s.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:5", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].StartupProbe = &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/healthz",
							Port: intstr.FromInt32(8080)}},
					PeriodSeconds: 10, FailureThreshold: 3,
				}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1", readyAfter(75*time.Second)),
				w.pod(1, "n2", readyAfter(75*time.Second)))
			c.after(time.Minute)
			setReplicas(w, 4)
			w.setReady(2)
			c.update(w.objects())
			killed := "Startup probe failed: Get \"http://10.244.0.12:8080" +
				"/healthz\": context deadline exceeded"
			for restarts := int32(1); restarts <= 5; restarts++ {
				for i, node := range []string{"n1", "n2"} {
					c.update(w.pod(i+2, node, startedNow, crashLoop(137,
						"Error", killed, restarts)))
				}
				c.after(50 * time.Second)
			}
		},
	}
}

// configProbe sets the container's port 8080 and a readiness probe on
// port.
func configProbe(spec *corev1.PodSpec, port int32) {
	spec.Containers[0].Ports = []corev1.ContainerPort{{
		Name: "http", ContainerPort: 8080,
	}}
	spec.Containers[0].ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/healthz", Port: intstr.FromInt32(port)}},
	}
}

// readyAfter is a running pod that became ready d after it started.
func readyAfter(d time.Duration) podState {
	return func(c *cluster, pod *corev1.Pod) {
		ready := metav1.NewTime(pod.Status.StartTime.Add(d))
		for i := range pod.Status.Conditions {
			switch pod.Status.Conditions[i].Type {
			case corev1.PodReady, corev1.ContainersReady:
				pod.Status.Conditions[i].LastTransitionTime = ready
			}
		}
	}
}
