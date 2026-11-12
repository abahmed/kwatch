package scenarios

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// pullQPSScenarios are image pulls a node's own kubelet holds back under
// its pull rate limit, which no registry refused.
func pullQPSScenarios() []scenario {
	return []scenario{kubeletPullQPSExceeded()}
}

// kubeletPullQPSExceeded: a new node takes a replica of three workloads
// at once. Its kubelet holds the pulls back under its own pull rate
// limit ("pull QPS exceeded") for four minutes and retries them; the
// same images run on the other nodes. The node's kubelet is the cause,
// not the registry, and it is no outage to page for.
func kubeletPullQPSExceeded() scenario {
	return scenario{
		expect: expectation{
			Name: "kubelet-pull-qps-exceeded",
			Description: "Pods of three workloads on a new node wait " +
				"because its kubelet answers \"pull QPS exceeded\"; " +
				"the registry serves the other nodes.",
			Root: "node//n3", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"registry//" + clusterRegistry,
				"deployment/shop/api", "deployment/shop/worker",
				"deployment/billing/ledger"},
		},
		build: buildKubeletPullQPS,
	}
}

func buildKubeletPullQPS(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	apps := []*workload{
		c.deployment("shop", "api", clusterRegistry+"/shop/api:4.2", 2),
		c.deployment("shop", "worker", clusterRegistry+"/shop/worker:4.2",
			2),
		c.deployment("billing", "ledger",
			clusterRegistry+"/billing/ledger:1.7", 2),
	}
	for _, w := range apps {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(time.Minute)
	n3 := c.node("n3", "zone-a")
	n3.CreationTimestamp = metav1.NewTime(c.now.Add(-time.Hour))
	c.create(n3)
	for _, w := range apps {
		setReplicas(w, 3)
		w.setReady(2)
		c.update(w.objects())
	}
	for n := range 6 {
		for _, w := range apps {
			throttledPullAttempt(c, w, 2, "n3", n)
		}
		c.after(40 * time.Second)
	}
}

// throttledPullAttempt is one round of the kubelet's pull cycle for a
// pull it holds back under its own rate limit: ErrImagePull with "pull
// QPS exceeded", then back-off, each failure also a Failed event.
func throttledPullAttempt(
	c *cluster, w *workload, i int, node string, attempt int,
) {
	image := w.deployment.Spec.Template.Spec.Containers[0].Image
	state := waiting("ErrImagePull", "pull QPS exceeded")
	if attempt%2 == 1 {
		state = waiting("ImagePullBackOff",
			"Back-off pulling image \""+image+"\"")
	}
	pod := w.pod(i, node, startedNow, state)
	pod.CreationTimestamp = metav1.NewTime(c.now.Add(
		-time.Duration(attempt) * 40 * time.Second))
	c.update(pod)
	if attempt%2 == 0 {
		c.warn(c.warningEvent(pod, "Pod", "Failed",
			"Failed to pull image \""+image+"\": pull QPS exceeded",
			"kubelet", int32(attempt/2+1)))
	}
}
