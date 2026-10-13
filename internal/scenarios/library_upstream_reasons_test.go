package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// upstreamReasonScenarios cover reasons the upstream snapshot
// (internal/detection/upstream) made kwatch classify: a waiting state it
// has no name for, a registry that cannot be reached, and a pod that
// cannot start because of an event the kubelet reports.
func upstreamReasonScenarios() []scenario {
	return []scenario{unknownWaitingReason(), registryUnavailable(),
		podContainerCreateFails()}
}

// holdPodIn runs a one-replica Deployment whose pod holds the given
// state for several minutes. each runs after every update.
func holdPodIn(
	c *cluster, name string, state podState, each func(pod *corev1.Pod),
) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", name, "registry.example.com/"+name+":3", 1)
	c.list(w.objects())
	c.after(time.Minute)
	for range 8 {
		pod := w.pod(0, "n1", state)
		c.update(pod)
		if each != nil {
			each(pod)
		}
		w.setReady(0)
		c.update(w.objects())
		c.after(45 * time.Second)
	}
}

// unknownWaitingReason: the kubelet reports a waiting reason kwatch has
// no name for. It is neither progress nor a known failure, so after its
// grace it is reported as a failure that quotes the state, not assumed
// healthy.
func unknownWaitingReason() scenario {
	return scenario{
		expect: expectation{
			Name: "unknown-waiting-reason",
			Description: "A container waits in a state kwatch has no name " +
				"for; once it lasts it is a failure quoting the state.",
			Root: "deployment/shop/ledger", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			holdPodIn(c, "ledger", waiting("WaitingOnSomethingNew",
				"the runtime is waiting on something new"), nil)
		},
	}
}

// registryUnavailable: the kubelet cannot reach the image registry and
// says so as the waiting reason itself.
func registryUnavailable() scenario {
	return scenario{
		expect: expectation{
			Name: "registry-unavailable",
			Description: "A container waits with the RegistryUnavailable " +
				"reason: a failure of the image pull, not progress.",
			Root: "deployment/shop/invoices", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			holdPodIn(c, "invoices", waiting("RegistryUnavailable",
				"Get \"https://registry.example.com/v2/\": dial tcp: "+
					"i/o timeout"), nil)
		},
	}
}

// podContainerCreateFails: the kubelet cannot create the pod's cgroup,
// which the pod's own state does not show: it only waits to be created.
func podContainerCreateFails() scenario {
	return scenario{
		expect: expectation{
			Name: "pod-cgroup-create-fails",
			Description: "The kubelet reports FailedCreatePodContainer " +
				"while the pod waits in ContainerCreating.",
			Root: "deployment/shop/search", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			holdPodIn(c, "search", waiting("ContainerCreating", ""),
				func(pod *corev1.Pod) {
					c.warn(c.warningEvent(pod, "Pod",
						"FailedCreatePodContainer",
						"unable to ensure pod container exists: failed to "+
							"create container for [kubepods burstable]: "+
							"mkdir /sys/fs/cgroup/kubepods: no space left "+
							"on device", "kubelet", 1))
				})
		},
	}
}
