package scenarios

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// heldoutPlacementScenarios are held-out pods that cannot be placed or
// admitted. They were written on 2026-10-01, when four earlier held-out
// scenarios became labelled, and their labels were fixed before they
// were first replayed. See heldoutLibrary for the rule they follow.
func heldoutPlacementScenarios() []scenario {
	return []scenario{heldoutStatefulSetZoneConflict(),
		heldoutQuotaPodCount()}
}

// heldoutStatefulSetZoneConflict: the kafka StatefulSet runs one broker
// per zone, each on a zonal volume. Its operator restarts broker kafka-2
// (the pod is deleted, nothing in its spec changes). kafka-2's volume
// lives in zone-a, and the only zone-a node has no memory left for it;
// every other node is in another zone. The scheduler reports "1
// Insufficient memory, 4 node(s) had volume node affinity conflict" and
// the broker stays Pending. A person names the claim whose volume is
// pinned to a zone without room; the StatefulSet and the nodes of other
// zones are only where it shows.
func heldoutStatefulSetZoneConflict() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-statefulset-zone-conflict",
			Description: "A restarted StatefulSet pod cannot be scheduled: " +
				"its zonal volume is in a zone whose only node is full.",
			Root: "persistentvolumeclaim/streaming/data-kafka-2",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"statefulset/streaming/kafka",
				"node//b1", "node//b2", "node//c1", "node//c2"},
		},
		build: buildHeldoutZoneConflict,
	}
}

func buildHeldoutZoneConflict(c *cluster) {
	nodes := map[string]string{"a1": "zone-a", "b1": "zone-b",
		"b2": "zone-b", "c1": "zone-c", "c2": "zone-c"}
	for _, name := range []string{"a1", "b1", "b2", "c1", "c2"} {
		c.list(c.node(name, nodes[name]))
	}
	c.list(scheduleClass(c, "zonal-ssd", "WaitForFirstConsumer"))
	sts := heldoutKafka(c)
	c.list(sts)
	homes := []string{"b1", "c1", "a1"}
	zones := []string{"zone-b", "zone-c", "zone-a"}
	for i, node := range homes {
		claim := storageBoundClaim(c, "streaming",
			fmt.Sprintf("data-kafka-%d", i), "zonal-ssd", "100Gi",
			fmt.Sprintf("pv-kafka-%d", i))
		c.list(heldoutZonalVolume(c, claim, zones[i]), claim)
		c.list(heldoutBroker(c, sts, i, node))
	}
	c.after(4 * time.Minute)
	c.remove(heldoutBroker(c, sts, 2, "a1"))
	sts = sts.DeepCopy()
	sts.Status.ReadyReplicas, sts.Status.AvailableReplicas = 2, 2
	c.update(sts)
	message := "0/5 nodes are available: 1 Insufficient memory, 4 " +
		"node(s) had volume node affinity conflict. preemption: 0/5 " +
		"nodes are available: 1 No preemption victims found for " +
		"incoming pod, 4 Preemption is not helpful for scheduling."
	pending := heldoutBroker(c, sts, 2, "", pendingUnscheduled(message))
	c.create(pending)
	for m := int32(1); m <= 6; m++ {
		c.after(time.Minute)
		c.warn(c.warningEvent(last(c, pending), "Pod", "FailedScheduling",
			message, "default-scheduler", m))
	}
}

// heldoutKafka is the three-broker StatefulSet of streaming, each broker
// mounting its own data-kafka-<i> claim.
func heldoutKafka(c *cluster) *appsv1.StatefulSet {
	meta := clusterMeta(c, "streaming", "kafka", "sts")
	replicas := int32(3)
	labels := map[string]string{"app": meta.Name}
	sts := &appsv1.StatefulSet{
		ObjectMeta: meta,
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas, ServiceName: meta.Name,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/kafka:3.8"),
		},
	}
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	sts.Status.Replicas, sts.Status.ReadyReplicas = 3, 3
	sts.Status.AvailableReplicas, sts.Status.CurrentReplicas = 3, 3
	sts.Status.UpdatedReplicas = 3
	return sts
}

// heldoutBroker is broker i of the kafka StatefulSet, mounting its own
// claim, on node; an empty node leaves it unscheduled.
func heldoutBroker(c *cluster, sts *appsv1.StatefulSet, i int,
	node string, states ...podState) *corev1.Pod {
	yes := true
	name := fmt.Sprintf("%s-%d", sts.Name, i)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: sts.Namespace, UID: types.UID(name),
			Labels:            sts.Spec.Template.Labels,
			CreationTimestamp: metav1.NewTime(c.start.Add(-time.Hour)),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "StatefulSet", Name: sts.Name,
				UID: sts.UID, Controller: &yes,
			}},
		},
		Spec: *sts.Spec.Template.Spec.DeepCopy(),
	}
	pod.Spec.Volumes = []corev1.Volume{{
		Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: c.n(fmt.Sprintf("data-kafka-%d", i)),
			},
		},
	}}
	if node != "" {
		pod.Spec.NodeName = c.n(node)
	}
	running(c, pod)
	for _, state := range states {
		state(c, pod)
	}
	return pod
}

// heldoutZonalVolume is claim's bound volume, reachable only from zone.
func heldoutZonalVolume(c *cluster, claim *corev1.PersistentVolumeClaim,
	zone string) *corev1.PersistentVolume {
	volume := storageVolume(c, claim, claim.Spec.VolumeName)
	volume.ObjectMeta = c.meta("", claim.Spec.VolumeName)
	volume.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
		Required: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      corev1.LabelTopologyZone,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{c.n(zone)},
				}},
			}},
		},
	}
	return volume
}

// heldoutQuotaPodCount: the ci namespace caps its pod count at 20 with
// a ResourceQuota. The build runners scale from 18 to 24 for a release;
// two new runners start, and every further create is forbidden with
// "exceeded quota: object-counts, requested: pods=1, used: pods=20". In
// the web namespace a Deployment scales out on the same nodes without
// trouble. The quota is the root.
func heldoutQuotaPodCount() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-quota-pod-count",
			Description: "A Deployment scale-up exceeds its namespace's " +
				"pod-count ResourceQuota while another namespace scales " +
				"out normally.",
			Root: "resourcequota/ci/object-counts", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//r1", "node//r2",
				"deployment/ci/runner", "deployment/web/portal"},
		},
		build: buildHeldoutQuotaPodCount,
	}
}

func buildHeldoutQuotaPodCount(c *cluster) {
	nodes := []string{"r1", "r2"}
	c.list(c.node("r1", "zone-a"), c.node("r2", "zone-b"))
	runner := c.deployment("ci", "runner",
		"registry.example.com/ci-runner:9", 18)
	portal := c.deployment("web", "portal",
		"registry.example.com/portal:2", 2)
	c.list(runner.objects())
	for i := range 18 {
		c.list(runner.pod(i, nodes[i%2]))
	}
	c.list(portal.objects())
	c.list(portal.pod(0, "r1"), portal.pod(1, "r2"))
	c.list(heldoutPodQuota(c, 18))
	c.after(3 * time.Minute)
	setReplicas(portal, 3)
	portal.setReady(2)
	c.update(portal.objects())
	c.create(portal.pod(2, "r2", startedNow))
	portal.setReady(3)
	c.update(portal.objects())
	setReplicas(runner, 24)
	runner.setReady(18)
	c.update(runner.objects())
	c.create(runner.pod(18, "r1", startedNow),
		runner.pod(19, "r2", startedNow))
	runner.setReady(20)
	c.update(runner.objects())
	c.update(heldoutPodQuota(c, 20))
	for n := int32(1); n <= 5; n++ {
		message := fmt.Sprintf("Error creating: pods \"%s-%c%c\" is "+
			"forbidden: exceeded quota: %s, requested: pods=1, used: "+
			"pods=20, limited: pods=20", runner.replicaSet.Name,
			'p'+rune(n), 'x', c.n("object-counts"))
		c.warn(c.warningEvent(runner.replicaSet, "ReplicaSet",
			"FailedCreate", message, "replicaset-controller", n*4))
		c.after(time.Minute)
	}
}

// heldoutPodQuota is the ci object-count quota with used pods.
func heldoutPodQuota(c *cluster, used int64) *corev1.ResourceQuota {
	quota := clusterQuota(c, "0", "0")
	quota.ObjectMeta = clusterMeta(c, "ci", "object-counts", "quota")
	hard := corev1.ResourceList{
		corev1.ResourcePods: resource.MustParse("20"),
	}
	quota.Spec.Hard, quota.Status.Hard = hard, hard
	quota.Status.Used = corev1.ResourceList{
		corev1.ResourcePods: *resource.NewQuantity(used,
			resource.DecimalSI),
	}
	return quota
}
