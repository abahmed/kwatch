package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// usageHistoryScenarios are failures explained by what the container
// used before it failed.
func usageHistoryScenarios() []scenario {
	return []scenario{oomSteadyClimb()}
}

// oomSteadyClimb: an API's memory climbs for three hours up to its
// 512Mi limit and the container is killed. kwatch's kubelet history
// says so, and the message names the climb and advises on the limit.
func oomSteadyClimb() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-steady-climb",
			Description: "A container's memory rose steadily from 200Mi " +
				"to its 512Mi limit over three hours before it was " +
				"OOM-killed.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			Tail: duration(10 * time.Minute),
		},
		build: buildOOMClimb,
	}
}

func buildOOMClimb(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:2.0", 1)
	w.setReady(0)
	c.list(w.objects())
	c.after(time.Minute)
	pod := w.pod(0, "n1", crashLoop(137, "OOMKilled", "", 4), longRun(3*time.Hour))
	c.list(pod)
	mib := float64(1 << 20)
	c.emit(inventory.Observation{
		Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
		Entity: kube.ContainerID(pod.Namespace, pod.Name, "app"),
		Attributes: map[string]inventory.Value{
			kube.AttrMemoryPeak24h:      inventory.Number(512 * mib),
			kube.AttrMemoryPrevStart:    inventory.Number(200 * mib),
			kube.AttrMemoryPrevPeak:     inventory.Number(512 * mib),
			kube.AttrMemoryPrevEnded:    inventory.Time(c.now.Add(-30 * time.Second)),
			kube.AttrMemoryPrevSeconds:  inventory.Number(3 * 3600),
			kube.AttrMemoryPrevDrawdown: inventory.Number(2),
		},
	})
	c.after(time.Minute)
}

// longRun makes the container's last run start ran ago and end ten
// seconds ago, with a 512Mi memory limit.
func longRun(ran time.Duration) podState {
	return func(c *cluster, pod *corev1.Pod) {
		status := &pod.Status.ContainerStatuses[0]
		terminated := status.LastTerminationState.Terminated
		terminated.StartedAt = metav1.NewTime(c.now.Add(-ran))
		pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		}
	}
}
