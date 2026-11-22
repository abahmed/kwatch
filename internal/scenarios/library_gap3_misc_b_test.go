package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap3MiscBScenarios are readiness gates, devices, autoscalers, peers'
// names, DNS access and notices that must stay quiet.
func gap3MiscBScenarios() []scenario {
	return []scenario{
		gap3ReadinessGate(), gap3DevicePlugin(), gap3HPAAmbiguous(),
		gap3HPAContainerMetric(), gap3ConflictingPod(), gap3HeadlessNotReady(),
		gap3CoreDNSRBAC(), gap3Hairpin(),
		gap3MountBlip(), gap3NameserverLimit(), gap3DNSLatency(),
	}
}

// gap3ReadinessGate: a release adds a readiness gate for a condition a
// removed load balancer controller used to set; the containers are ready
// but the pod never is.
func gap3ReadinessGate() scenario {
	return g2Release(g2Exp("release-readiness-gate-never-set",
		"A release adds a readiness gate whose controller no longer "+
			"runs; the new pod's containers are ready but the pod "+
			"never becomes Ready and the rollout stalls.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.ReadinessGates = []corev1.PodReadinessGate{{
				ConditionType: "target-health.elbv2.k8s.aws/api"}}
		},
		func(c *cluster, w *workload) {
			gapRounds(c, w, 8, time.Minute, func(int32) podState {
				return func(c *cluster, pod *corev1.Pod) {
					for i := range pod.Status.Conditions {
						if pod.Status.Conditions[i].Type == corev1.PodReady {
							pod.Status.Conditions[i].Status =
								corev1.ConditionFalse
							pod.Status.Conditions[i].Reason =
								"ReadinessGatesNotReady"
							pod.Status.Conditions[i].LastTransitionTime =
								metav1.NewTime(c.now)
						}
					}
				}
			}, nil)
		})
}

// gap3DevicePlugin: the device plugin DaemonSet pod on n3 crash-loops; the
// node's GPU allocatable falls to zero, the restarted trainer pod is
// rejected there and replacements stay Pending.
func gap3DevicePlugin() scenario {
	return scenario{
		expect: g2Exp("device-plugin-down-gpu-unavailable",
			"The GPU device plugin crash-loops on a node; its GPU "+
				"allocatable drops to zero, a restarted pod is rejected "+
				"there and new pods stay Pending on \"Insufficient "+
				"nvidia.com/gpu\".",
			"daemonset/kube-system/nvidia-device-plugin", "notify",
			"deployment/ml/trainer", "node//n3").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			gpu := corev1.ResourceName("nvidia.com/gpu")
			n3 := c.node("n3", "zone-a")
			n3.Status.Allocatable[gpu] = resource.MustParse("2")
			c.list(c.node("n1", "zone-a"), n3)
			ds := g2Daemon(c, "kube-system", "nvidia-device-plugin", "n3")
			w := c.deployment("ml", "trainer", "registry.example.com/t:3", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n3"))
			c.after(5 * time.Minute)
			crash := "failed to start device plugin: could not load NVML: " +
				"Driver/library version mismatch"
			for restarts := int32(1); restarts <= 3; restarts++ {
				setAgentCounts(ds, 1, 0)
				c.update(ds, agentPod(c, ds, "n3", crashLoop(1, "Error",
					crash, restarts)))
				c.after(time.Minute)
			}
			down := last(c, n3)
			down.Status.Allocatable[gpu] = resource.MustParse("0")
			c.update(down)
			message := "Allocate failed due to no healthy devices " +
				"present; cannot allocate unhealthy devices nvidia.com/" +
				"gpu, which is unexpected"
			bad := w.pod(1, "n3", g2Failed("UnexpectedAdmissionError",
				message))
			c.update(bad)
			gapKubelet(c, bad, "UnexpectedAdmissionError", message, 1)
			pending := "0/2 nodes are available: 1 Insufficient nvidia." +
				"com/gpu, 1 node(s) didn't match Pod's node affinity/" +
				"selector."
			c.create(w.pod(2, "", pendingUnscheduled(pending)))
			for m := int32(1); m <= 5; m++ {
				c.after(time.Minute)
				g2Event(c, last(c, w.pod(2, "")), "Pod", "FailedScheduling",
					pending, "default-scheduler", m)
			}
		},
	}
}

// gap3HPAAmbiguous: a second HPA is created for the Deployment web; both
// report that the pods are controlled by several autoscalers and stop
// scaling.
func gap3HPAAmbiguous() scenario {
	e := g2Exp("hpa-duplicate-for-one-deployment",
		"A second HPA is created for a Deployment; both report "+
			"AmbiguousSelector and neither scales.",
		"horizontalpodautoscaler/shop/web-extra", "digest", "node//n1").
		tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{
		expect: e,
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.list(clusterHPA(c, "shop", "web", "True", "ValidMetricFound",
				""))
			c.after(10 * time.Minute)
			message := "pods by selector app=" + c.n("web") + " are " +
				"controlled by multiple HPAs: [shop/" + c.n("web") +
				" shop/" + c.n("web-extra") + "]"
			for m := int32(1); m <= 4; m++ {
				for _, name := range []string{"web", "web-extra"} {
					hpa := clusterHPA(c, "shop", name, "False",
						"AmbiguousSelector", message)
					hpa.Spec.ScaleTargetRef.Name = c.n("web")
					c.update(hpa)
					g2Event(c, hpa, "HorizontalPodAutoscaler",
						"AmbiguousSelector", message,
						"horizontal-pod-autoscaler", m)
				}
				c.after(5 * time.Minute)
			}
		},
	}
}

// gap3HPAContainerMetric: an HPA scales on the CPU of a container named
// app; the pods now call it api.
func gap3HPAContainerMetric() scenario {
	return scenario{
		expect: g2Exp("hpa-container-metric-name-missing",
			"An HPA's containerResource metric names a container that "+
				"the pods no longer have; it cannot scale.",
			"horizontalpodautoscaler/shop/web", "digest", "node//n1",
			"apiservice//v1beta1.metrics.k8s.io").tail(20 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.list(clusterAPIService(c, "True", "Passed",
				"all checks passed"))
			message := "failed to get cpu container metric value: " +
				"container 'app' not present in metrics for pod " +
				w.pod(0, "n1").Namespace + "/" + w.pod(0, "n1").Name
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				hpa := clusterHPA(c, "shop", "web", "False",
					"FailedGetContainerResourceMetric", message)
				c.update(hpa)
				g2Event(c, hpa, "HorizontalPodAutoscaler",
					"FailedGetContainerResourceMetric", message,
					"horizontal-pod-autoscaler", m)
			}
		},
	}
}

// gap3ConflictingPod: someone created a bare pod called db-1 by hand; the
// StatefulSet db cannot create its own db-1.
func gap3ConflictingPod() scenario {
	return scenario{
		expect: g2Exp("statefulset-ordinal-name-taken-by-bare-pod",
			"A StatefulSet cannot create its pod db-1 because a pod "+
				"with that name exists that it does not own.",
			"pod/data/db-1", "digest", "node//n1",
			"statefulset/data/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			meta := clusterMeta(c, "data", "db", "sts")
			labels := map[string]string{"app": meta.Name}
			replicas := int32(3)
			sts := &appsv1.StatefulSet{ObjectMeta: meta,
				Spec: appsv1.StatefulSetSpec{Replicas: &replicas,
					ServiceName: meta.Name,
					Selector:    &metav1.LabelSelector{MatchLabels: labels},
					Template:    podTemplate(labels, "registry.example.com/db:5"),
				},
				Status: appsv1.StatefulSetStatus{Replicas: 1,
					ReadyReplicas: 1}}
			sts.Generation, sts.Status.ObservedGeneration = 1, 1
			c.list(sts)
			c.list(operatorBroker(c, sts, 0, "n1"))
			bare := operatorBroker(c, sts, 1, "n1")
			bare.OwnerReferences = nil
			c.list(bare)
			for m := int32(1); m <= 5; m++ {
				c.after(2 * time.Minute)
				g2Event(c, sts, "StatefulSet", "ConflictingController",
					"pod data/"+bare.Name+" is not controlled by "+
						"StatefulSet "+sts.Name, "statefulset-controller",
					m)
			}
		},
	}
}

// gap3CoreDNSRBAC: an upgrade replaces the system:coredns ClusterRole with
// one that cannot list EndpointSlices; CoreDNS runs but cannot answer for
// Services, and apps of several namespaces fail their lookups.
func gap3CoreDNSRBAC() scenario {
	return scenario{
		expect: g2Exp("coredns-cluster-role-edited",
			"The ClusterRole CoreDNS uses loses access to "+
				"EndpointSlices; CoreDNS pods stay Running but lookups "+
				"of Services fail in several namespaces and the DNS "+
				"probe fails.",
			"clusterrole//system:coredns", "page", "node//n1", "node//n2",
			"deployment/shop/cart", "deployment/billing/invoices"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
			role := func(resources ...any) *unstructured.Unstructured {
				return rbacObject(c, "ClusterRole", "", "system:coredns",
					map[string]any{"rules": []any{map[string]any{
						"apiGroups": []any{"discovery.k8s.io"},
						"resources": resources,
						"verbs":     []any{"list", "watch"}}}})
			}
			dns := c.deployment("kube-system", "coredns",
				"registry.k8s.io/coredns/coredns:v1.11.1", 2)
			configTemplate(dns, func(spec *corev1.PodSpec) {
				spec.ServiceAccountName = "coredns"
			})
			binding := rbacObject(c, "ClusterRoleBinding", "",
				"system:coredns", map[string]any{
					"subjects": []any{map[string]any{
						"kind": "ServiceAccount", "name": "coredns",
						"namespace": c.n("kube-system")}},
					"roleRef": map[string]any{
						"apiGroup": "rbac.authorization.k8s.io",
						"kind":     "ClusterRole", "name": "system:coredns"}})
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:3.1", 2)
			invoices := c.deployment("billing", "invoices",
				"registry.example.com/invoices:1.8", 2)
			c.list(role("endpointslices"), binding)
			for _, w := range []*workload{dns, cart, invoices} {
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			}
			c.probe(kube.ClusterDNS, "")
			c.after(10 * time.Minute)
			c.update(role("pods"))
			c.after(time.Minute)
			for round := int32(1); round <= 6; round++ {
				c.probe(kube.ClusterDNS, "lookup kubernetes.default.svc."+
					"cluster.local on 10.96.0.10:53: server misbehaving")
				if round >= 2 {
					clusterDNSVictims(c, cart, "db.shop.svc.cluster.local",
						round-1)
					clusterDNSVictims(c, invoices,
						"payments-api.billing.svc.cluster.local", round-1)
				}
				c.after(30 * time.Second)
			}
		},
	}
}

// gap3HeadlessNotReady: a clustered StatefulSet's peers can only find each
// other by name; the headless Service does not publish not-ready
// addresses, so no peer resolves and none becomes ready.
func gap3HeadlessNotReady() scenario {
	return scenario{
		expect: g2Exp("headless-service-hides-not-ready-peers",
			"A clustered StatefulSet's pods wait for their peers' names "+
				"to resolve, but the headless Service publishes only "+
				"ready addresses; no pod becomes ready.",
			"service/streaming/queue", "notify", "node//n1",
			"statefulset/streaming/queue").tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			svc := clusterService(c, "streaming", "queue", 9092)
			svc.Spec.ClusterIP = "None"
			svc.Spec.ClusterIPs = []string{"None"}
			c.list(svc, clusterSlice(c, "streaming", "queue"))
			c.after(10 * time.Minute)
			updated := sts.DeepCopy()
			updated.Status.ReadyReplicas = 0
			updated.Status.AvailableReplicas = 0
			c.update(updated)
			for round := int32(1); round <= 8; round++ {
				for i := range 3 {
					pod := operatorBroker(c, sts, i,
						"n"+itoa(i%2+1), notReady)
					c.update(pod)
					gapKubelet(c, pod, "Unhealthy", "Readiness probe "+
						"failed: cluster not formed: cannot resolve "+
						"queue-"+itoa(i+1)+".queue.streaming.svc: "+
						"NXDOMAIN", round)
				}
				c.after(time.Minute)
			}
		},
	}
}
