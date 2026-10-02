package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// lifecycleScenarios exercise incident lifecycles: resource limits,
// flapping, simultaneous independent problems, revised causes and cold
// starts.
func lifecycleScenarios() []scenario {
	return []scenario{
		oomLimitTooLow(), flappingWorkload(), twoIndependentProblems(),
		causeRevised(), coldStartPreexisting(),
	}
}

// lifecycleMemoryLimit gives the workload's container a memory limit.
func lifecycleMemoryLimit(limit string) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) {
		spec.Containers[0].Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse(limit),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse(limit),
			},
		}
	}
}

// oomLimitTooLow: every replica is OOM-killed on healthy nodes because
// its 64Mi limit is below what the application needs. The workload's own
// configuration is the root; the nodes are fine.
func oomLimitTooLow() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-limit-too-low",
			Description: "Both replicas of a Deployment with a 64Mi " +
				"memory limit are OOMKilled (exit 137) again and again on " +
				"healthy nodes without memory pressure.",
			Root: "deployment/shop/cart", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{
				"node//n1", "node//n2", "zone//zone-a",
			},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "cart",
				"registry.example.com/cart:5.0", 2)
			w.rollout(lifecycleMemoryLimit("64Mi"))
			w.setReady(2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.after(time.Minute)
				for i, node := range []string{"n1", "n2"} {
					c.update(w.pod(i, node, crashLoop(137, "OOMKilled",
						"", restarts)))
				}
				if restarts == 1 {
					w.setReady(0)
					c.update(w.objects())
				}
			}
		},
	}
}

// flappingWorkload: one replica keeps crashing and recovering for an
// hour. The workload is the root; each cycle must not be a new message.
func flappingWorkload() scenario {
	return scenario{
		expect: expectation{
			Name: "flapping-workload",
			Description: "One replica of a Deployment crash-loops for a " +
				"few minutes, recovers, and crashes again, six times in " +
				"an hour; the other replica stays healthy.",
			Root: "deployment/shop/search", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(2 * time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "search",
				"registry.example.com/search:7.4", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			restarts := int32(0)
			for cycle := 0; cycle < 6; cycle++ {
				c.after(4 * time.Minute)
				for step := 0; step < 3; step++ {
					restarts++
					c.update(w.pod(0, "n1", crashLoop(2, "Error",
						"fatal: connection pool exhausted", restarts)))
					w.setReady(1)
					c.update(w.objects())
					c.after(90 * time.Second)
				}
				c.update(w.pod(0, "n1", lifecycleRestarted(restarts)))
				w.setReady(2)
				c.update(w.objects())
			}
		},
	}
}

// lifecycleRestarted is a running, ready container that restarted. Like
// the kubelet, it keeps reporting the run that ended before the restart.
func lifecycleRestarted(restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		startedNow(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 2, Reason: "Error",
				Message:    "fatal: connection pool exhausted",
				StartedAt:  metav1.NewTime(c.now.Add(-20 * time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-5 * time.Second)),
			},
		}
	}
}

// coldStartPreexisting: kwatch starts while two unrelated failures exist.
// The API pods crash on a bad configuration value, and the invoicer
// cannot start because its Secret was never created. The Secret is the
// invoicer's root; the API deployment is its own root.
func coldStartPreexisting() scenario {
	return scenario{
		expect: expectation{
			Name: "cold-start-preexisting",
			Description: "kwatch starts while one Deployment already " +
				"crash-loops and another cannot start because a Secret " +
				"it needs does not exist; both belong in one startup " +
				"summary.",
			Root: "secret/billing/invoicer-creds",
			OtherRoots: []string{
				"deployment/shop/api",
			},
			Tier: "notify", MaxMessages: 1,
			MustNotBlame: []string{
				"node//n1", "node//n2", "zone//zone-a",
			},
			SyncAfter: duration(30 * time.Second),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			api := c.deployment("shop", "api",
				"registry.example.com/api:9.1", 2)
			api.setReady(0)
			c.list(api.objects())
			for i, node := range []string{"n1", "n2"} {
				c.list(api.pod(i, node, crashLoop(1, "Error",
					"invalid value for LISTEN_PORT: \"80a\"", 14)))
			}
			inv := c.deployment("billing", "invoicer",
				"registry.example.com/invoicer:2.0", 1)
			inv.rollout(lifecycleSecretEnv(c.n("invoicer-creds")))
			inv.setReady(0)
			c.list(inv.objects())
			pod := inv.pod(0, "n2", waiting("CreateContainerConfigError",
				"secret \""+c.n("invoicer-creds")+"\" not found"))
			c.list(pod)
			c.after(10 * time.Second)
			c.warn(c.warningEvent(pod, "Pod", "Failed",
				"Error: secret \""+c.n("invoicer-creds")+"\" not found",
				"kubelet", 40))
		},
	}
}

// lifecycleSecretEnv makes the container read a key of a Secret.
func lifecycleSecretEnv(secret string) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "STRIPE_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret,
					},
					Key: "api-key",
				},
			},
		}}
	}
}
