package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rcaDiskHogScenarios are faults that a workload carries from node to
// node.
func rcaDiskHogScenarios() []scenario {
	return []scenario{diskHogMovesNodes()}
}

// diskHogMovesNodes: ingest writes tens of gigabytes into an emptyDir
// and has no ephemeral-storage limit. The kubelet of n1 reports
// DiskPressure and evicts it; its replacement lands on n2 and fills
// that disk too, seven minutes later, and is evicted again. Two nodes
// show pressure one after the other, but one workload is the cause of
// both and one person has one thing to fix.
func diskHogMovesNodes() scenario {
	return scenario{
		expect: expectation{
			Name: "disk-hog-moves-nodes",
			Description: "One workload without an ephemeral-storage " +
				"limit fills the disk of one node, is evicted, and " +
				"fills the disk of the node it moves to.",
			Root: "deployment/media/ingest", Tier: "notify",
			MaxMessages: 4, MaxPages: 1,
			// The first node's pressure is told as the node's until
			// the workload shows up on a second node.
			RevisesRoot: true,
			MustNotBlame: []string{"node//n3", "deployment/media/thumbs",
				"deployment/media/api", "zone//zone-a"},
			Tail: duration(15 * time.Minute),
		},
		build: buildDiskHog,
	}
}

func rcaDiskPressure(
	c *cluster, node *corev1.Node, on bool, at time.Time,
) *corev1.Node {
	out := last(c, node)
	status, reason := corev1.ConditionFalse, "KubeletHasNoDiskPressure"
	out.Spec.Taints = nil
	if on {
		status, reason = corev1.ConditionTrue, "KubeletHasDiskPressure"
		out.Spec.Taints = []corev1.Taint{{
			Key: "node.kubernetes.io/disk-pressure", Effect: "NoSchedule",
		}}
	}
	setNodeCondition(out, corev1.NodeDiskPressure, status, reason,
		"kubelet disk state", at)
	return out
}

// rcaEvictForDisk is the kubelet evicting replica i of w from node for
// its ephemeral-storage use, with the usual message.
func rcaEvictForDisk(c *cluster, w *workload, i int, node string, gi int) {
	message := fmt.Sprintf("The node was low on resource: "+
		"ephemeral-storage. Threshold quantity: 10%%, available: 3%%. "+
		"Container app was using %dGi, request is 0, has larger "+
		"consumption of ephemeral-storage.", gi)
	pod := w.pod(i, node, evicted(message))
	rcaEphemeralUsed(c, pod, gi)
	nodeDisruption(c, pod, "TerminationByKubelet",
		"The node was low on resource: ephemeral-storage.")
	c.update(pod)
	c.warn(c.warningEvent(pod, "Pod", "Evicted", message, "kubelet", 1))
}

func buildDiskHog(c *cluster) {
	n1, n2 := c.node("n1", "zone-a"), c.node("n2", "zone-a")
	c.list(n1, n2, c.node("n3", "zone-a"))
	nodeFleet(c, "media", []string{"thumbs", "api"}, []string{"n1", "n2"})
	ingest := c.deployment("media", "ingest",
		"registry.example.com/ingest:4.2", 1)
	configTemplate(ingest, func(spec *corev1.PodSpec) {
		spec.Volumes = []corev1.Volume{{Name: "scratch",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	})
	c.list(ingest.objects())
	c.list(ingest.pod(0, "n1"))
	c.after(2 * time.Minute)
	c.update(rcaDiskPressure(c, n1, true, c.now))
	c.warn(c.warningEvent(n1, "Node", "EvictionThresholdMet",
		"Attempting to reclaim ephemeral-storage", "kubelet", 1))
	c.after(15 * time.Second)
	rcaEvictForDisk(c, ingest, 0, "n1", 41)
	ingest.setReady(0)
	c.update(ingest.objects())
	c.after(20 * time.Second)
	c.create(ingest.pod(1, "n2", startedNow))
	ingest.setReady(1)
	c.update(ingest.objects())
	c.after(2 * time.Minute)
	c.update(rcaDiskPressure(c, n1, false, c.now))
	c.after(5 * time.Minute)
	c.update(rcaDiskPressure(c, n2, true, c.now))
	c.warn(c.warningEvent(n2, "Node", "EvictionThresholdMet",
		"Attempting to reclaim ephemeral-storage", "kubelet", 1))
	c.after(15 * time.Second)
	rcaEvictForDisk(c, ingest, 1, "n2", 40)
	ingest.setReady(0)
	c.update(ingest.objects())
	c.after(20 * time.Second)
	c.create(ingest.pod(2, "n3", startedNow))
	ingest.setReady(1)
	c.update(ingest.objects())
	c.after(2 * time.Minute)
	c.update(rcaDiskPressure(c, n2, false, c.now))
	c.after(5 * time.Minute)
}

// rcaEphemeralUsed is the kubelet's last reading of the pod's
// ephemeral-storage use, in gibibytes, before it was evicted.
func rcaEphemeralUsed(c *cluster, pod *corev1.Pod, gi int) {
	c.emit(inventory.Observation{
		Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
		Entity: inventory.CoreID(kube.KindPod, pod.Namespace, pod.Name),
		Attributes: map[string]inventory.Value{
			kube.AttrEphemeralUsed: inventory.Number(
				float64(gi) * (1 << 30))},
	})
}
