package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// advCapacityScenarios are adversarial stories about limits: ceilings,
// quotas, placement, finalizers and flapping under load.
func advCapacityScenarios() []scenario {
	return []scenario{
		advHPAMaxThrottled(), advQuotaAfterHPA(), advStatefulSetNoZone(),
		advReadinessFlapLoad(),
	}
}

// advHPAMaxThrottled: web is CPU-throttled 80% of the time under a 200m
// limit, its HPA sits at maxReplicas 4 while wanting 8, and readiness
// probes time out. Nodes are fine. The ceiling is the root a person
// raises first.
func advHPAMaxThrottled() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-hpa-max-throttled",
			Description: "An HPA is at its maximum while its throttled " +
				"pods miss readiness deadlines.",
			Root: "horizontalpodautoscaler/shop/web", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"deployment/shop/web"},
		},
		build: func(c *cluster) {
			nodes := []string{"n1", "n2"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:9", 4)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].Resources.Limits = corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("200m"),
				}
			})
			c.list(w.objects())
			for i := range 4 {
				c.list(w.pod(i, nodes[i%2]))
			}
			c.list(autoscalerAtMax(c, "shop", "web", 4, 4, 4, 55, false))
			c.after(2 * time.Minute)
			c.update(autoscalerAtMax(c, "shop", "web", 4, 4, 8, 230, true))
			probe := "Readiness probe failed: Get \"http://10.244.0.9:" +
				"8080/healthz\": context deadline exceeded"
			for round := int32(1); round <= 5; round++ {
				for i := range 4 {
					pod := w.pod(i, nodes[i%2], notReady)
					c.update(pod)
					c.warn(c.warningEvent(pod, "Pod", "Unhealthy", probe,
						"kubelet", round*2))
					c.emit(inventory.Observation{
						Kind: inventory.Observed, Source: "cadvisor",
						At: c.now, Entity: kube.ContainerID(pod.Namespace,
							pod.Name, "app"),
						Attributes: map[string]inventory.Value{
							kube.AttrThrottledPct: inventory.Number(80)},
					})
				}
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

func advPodQuota(c *cluster, used int64) *corev1.ResourceQuota {
	quota := clusterQuota(c, "0", "0")
	quota.ObjectMeta = clusterMeta(c, "batch", "pod-cap", "quota")
	hard := corev1.ResourceList{
		corev1.ResourcePods: resource.MustParse("6"),
	}
	quota.Spec.Hard, quota.Status.Hard = hard, hard
	quota.Status.Used = corev1.ResourceList{
		corev1.ResourcePods: *resource.NewQuantity(used,
			resource.DecimalSI),
	}
	return quota
}

// advQuotaAfterHPA: the HPA of batch/worker scales out on CPU from 4 to
// 8; the namespace caps pods at 6, so the fifth and later creates are
// forbidden. The HPA is working and below its maximum; the quota is the
// root.
func advQuotaAfterHPA() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-quota-after-hpa-scale-up",
			Description: "An HPA scale-out runs into the namespace's pod " +
				"quota; the HPA is below its maximum.",
			Root: "resourcequota/batch/pod-cap", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"horizontalpodautoscaler/batch/worker",
				"deployment/batch/worker", "node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			nodes := []string{"n1", "n2"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("batch", "worker",
				"registry.example.com/worker:3", 4)
			c.list(w.objects())
			for i := range 4 {
				c.list(w.pod(i, nodes[i%2]))
			}
			c.list(advPodQuota(c, 4))
			c.list(autoscalerAtMax(c, "batch", "worker", 12, 4, 4, 55,
				false))
			c.after(2 * time.Minute)
			setReplicas(w, 8)
			w.setReady(4)
			c.update(w.objects())
			c.update(autoscalerAtMax(c, "batch", "worker", 12, 4, 8, 150,
				false))
			c.create(w.pod(4, "n1", startedNow), w.pod(5, "n2", startedNow))
			w.setReady(6)
			c.update(w.objects())
			c.update(advPodQuota(c, 6))
			for n := int32(1); n <= 5; n++ {
				message := fmt.Sprintf("Error creating: pods \"%s-x%d\" is "+
					"forbidden: exceeded quota: %s, requested: pods=1, "+
					"used: pods=6, limited: pods=6", w.replicaSet.Name, n,
					c.n("pod-cap"))
				c.warn(c.warningEvent(w.replicaSet, "ReplicaSet",
					"FailedCreate", message, "replicaset-controller", n*3))
				c.after(time.Minute)
			}
		},
	}
}

// advStatefulSetNoZone: broker kafka-2's volume is pinned to zone-c, and
// the cluster has no zone-c nodes. The scheduler and the cluster
// autoscaler both say it, the autoscaler adding that it will not scale
// up. The claim's zone is the root, not the autoscaler's refusal.
func advStatefulSetNoZone() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-statefulset-zone-without-nodes",
			Description: "A StatefulSet pod is Pending because its " +
				"volume lives in a zone with no nodes; the autoscaler " +
				"reports it will not scale up.",
			Root: "persistentvolumeclaim/streaming/data-kafka-2",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"statefulset/streaming/kafka",
				"node//a1", "node//b1", "node//b2"},
		},
		build: func(c *cluster) {
			zones := map[string]string{"a1": "zone-a", "b1": "zone-b",
				"b2": "zone-b"}
			for _, name := range []string{"a1", "b1", "b2"} {
				c.list(c.node(name, zones[name]))
			}
			c.list(scheduleClass(c, "zonal-ssd", "WaitForFirstConsumer"))
			sts := heldoutKafka(c)
			sts.Status.ReadyReplicas, sts.Status.AvailableReplicas = 2, 2
			c.list(sts)
			homes := []string{"a1", "b1", ""}
			volumeZones := []string{"zone-a", "zone-b", "zone-c"}
			for i := range 3 {
				claim := storageBoundClaim(c, "streaming",
					fmt.Sprintf("data-kafka-%d", i), "zonal-ssd", "100Gi",
					fmt.Sprintf("pv-kafka-%d", i))
				c.list(heldoutZonalVolume(c, claim, volumeZones[i]), claim)
				if homes[i] != "" {
					c.list(heldoutBroker(c, sts, i, homes[i]))
				}
			}
			message := "0/3 nodes are available: 3 node(s) had volume " +
				"node affinity conflict. preemption: 0/3 nodes are " +
				"available: 3 Preemption is not helpful for scheduling."
			pending := heldoutBroker(c, sts, 2, "",
				pendingUnscheduled(message))
			c.create(pending)
			for m := int32(1); m <= 7; m++ {
				c.after(time.Minute)
				pod := last(c, pending)
				c.warn(c.warningEvent(pod, "Pod", "FailedScheduling",
					message, "default-scheduler", m))
				c.warn(c.warningEvent(pod, "Pod", "NotTriggerScaleUp",
					"pod didn't trigger scale-up: 3 node(s) had volume "+
						"node affinity conflict", "cluster-autoscaler", m))
			}
		},
	}
}

// advReadinessFlapLoad: six API pods switch readiness off and on at
// staggered times for twenty-five minutes under load, without a single
// restart, and an unrelated Deployment of the namespace completes a
// healthy rollout in the middle. One workload flaps; one incident, no
// cause invented, no second message for each flip.
func advReadinessFlapLoad() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-readiness-flap-under-load",
			Description: "Six pods flap readiness for 25 minutes without " +
				"restarts while an unrelated rollout completes.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 3,
			MaxPages: 1,
			MustNotBlame: []string{"node//n1", "node//n2",
				"service/shop/api", "deployment/shop/web"},
			Tail: duration(10 * time.Minute),
		},
		build: buildAdvReadinessFlap,
	}
}

func buildAdvReadinessFlap(c *cluster) {
	nodes := []string{"n1", "n2"}
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:5", 6)
	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: "/ready",
			Port: intstr.FromInt(8080)}}, PeriodSeconds: 5}
	w.deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	w.replicaSet.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	web := c.deployment("shop", "web", "registry.example.com/web:3", 2)
	c.list(w.deployment, w.replicaSet, web.deployment, web.replicaSet)
	var pods []*corev1.Pod
	for i := range 6 {
		pods = append(pods, w.pod(i, nodes[i%2]))
	}
	c.list(pods[0], pods[1], pods[2], pods[3], pods[4], pods[5])
	c.list(web.pod(0, "n1"), web.pod(1, "n2"))
	c.after(20 * time.Minute)
	down := make([]bool, 6)
	for step := range 50 {
		replica := step % 6
		down[replica] = !down[replica]
		pod := w.pod(replica, nodes[replica%2])
		if down[replica] {
			pod = w.pod(replica, nodes[replica%2], notReady)
		}
		c.update(pod)
		ready := int32(6)
		for _, d := range down {
			if d {
				ready--
			}
		}
		w.setReady(ready)
		c.update(w.objects())
		if down[replica] {
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", flapProbe,
				"kubelet", int32(step+1)))
		}
		if step == 24 {
			old := []*corev1.Pod{web.pod(0, "n1"), web.pod(1, "n2")}
			rs := web.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/web:3.1"
			})
			c.update(web.deployment)
			c.create(rs)
			c.create(web.pod(0, "n1", startedNow), web.pod(1, "n2",
				startedNow))
			c.remove(old[0], old[1])
			web.setReady(2)
			c.update(web.objects())
		}
		c.after(30 * time.Second)
	}
}
