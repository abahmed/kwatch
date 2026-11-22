package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// gap18bScenarios: a slice of traffic, a tenant namespace, one port or
// one ordinal fails; the rest keeps serving.
func gap18bScenarios() []scenario {
	return []scenario{
		gap18CoreDNSUnsynced(), gap18Canary(), gap18DefaultDeny(),
		gap18StorageQuota(), gap18TopologySpread(), gap18RuntimeClass(),
		gap18OrdinalCorrupt(), gap18UDPBlocked(), gap18ReadinessGate(),
		gap18LocalPolicy(),
	}
}

// gap18CoreDNSUnsynced: one of two coredns replicas lost its list
// permission and never syncs.
func gap18CoreDNSUnsynced() scenario {
	return scenario{
		expect: g2Exp("intermittent-one-coredns-replica-unsynced",
			"One of two coredns replicas is Running but not Ready after "+
				"its ServiceAccount lost list endpointslices; half of the "+
				"DNS queries time out.", "deployment/kube-system/coredns",
			"notify").tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"n1", "n2"}
			for _, name := range place {
				c.list(c.node(name, "zone-a"))
			}
			w := g18Deploy(c, "kube-system", "coredns", place, nil)
			c.after(5 * time.Minute)
			pod := w.pod(1, "n2", notReady)
			c.update(pod)
			g10Event(c, pod, "Unhealthy", "Readiness probe failed: HTTP "+
				"probe failed with statuscode: 503 plugin/ready: "+
				"kubernetes not synced", "kubelet", 8, c.now)
			g10Settle(c, w, 1)
		},
	}
}

// gap18Canary: the one canary replica crash-loops while twenty stable
// replicas serve.
func gap18Canary() scenario {
	return scenario{
		expect: g2Exp("canary-five-percent-bad-image",
			"A single-replica canary Deployment crash-loops on image v4 "+
				"behind the same Service as twenty healthy stable "+
				"replicas; about 5% of requests fail.",
			"deployment/shop/api-canary", "notify",
			"deployment/shop/api").budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"n1", "n2", "n3", "n1", "n2", "n3", "n1", "n2",
				"n3", "n1", "n2", "n3", "n1", "n2", "n3", "n1", "n2", "n3",
				"n1", "n2"}
			for _, name := range gap10Nodes {
				c.list(c.node(name, "zone-a"))
			}
			g18Deploy(c, "shop", "api", place, nil)
			canary := g18Deploy(c, "shop", "api-canary", []string{"n1"}, nil)
			for restarts := int32(2); restarts <= 6; restarts += 2 {
				c.after(3 * time.Minute)
				g18Fail(c, canary, []string{"n1"}, []int{0}, crashLoop(2,
					"Error", "fatal: unsupported schema v4", restarts))
			}
		},
	}
}

// gap18DefaultDeny: a default-deny policy without a DNS rule breaks every
// pod of one tenant namespace.
func gap18DefaultDeny() scenario {
	return scenario{
		expect: g2Exp("tenant-default-deny-breaks-dns",
			"A new default-deny NetworkPolicy (ingress and egress, no DNS "+
				"rule) in tenant-b; its nine pods fail \"lookup db\" "+
				"timeouts and go unready; tenant-a and tenant-c are fine.",
			"networkpolicy/tenant-b/default-deny-all", "notify",
			"deployment/tenant-b/shop").tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"n1", "n2", "n3"}
			for _, name := range place {
				c.list(c.node(name, "zone-a"))
			}
			var tenantB []*workload
			for _, ns := range []string{"tenant-a", "tenant-c"} {
				g18Deploy(c, ns, "shop", place, nil)
			}
			for _, name := range []string{"shop", "cart", "search"} {
				tenantB = append(tenantB, g18Deploy(c, "tenant-b", name,
					place, nil))
			}
			c.after(10 * time.Minute)
			c.create(&networkingv1.NetworkPolicy{
				ObjectMeta: clusterMeta(c, "tenant-b", "default-deny-all",
					"netpol"),
				Spec: networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.
					PolicyType{networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress}}})
			c.after(2 * time.Minute)
			for _, w := range tenantB {
				for i, node := range place {
					pod := w.pod(i, node, notReady)
					c.update(pod)
					g10Event(c, pod, "Unhealthy", "Readiness probe failed: "+
						"lookup db: i/o timeout", "kubelet", 3, c.now)
				}
				g10Settle(c, w, 0)
			}
		},
	}
}

// gap18StorageQuota: a zero storage quota for one class keeps the first
// ordinal's claim from being created.
func gap18StorageQuota() scenario {
	return scenario{
		expect: g2Exp("tenant-quota-forbids-storageclass",
			"tenant-c's ResourceQuota allows zero storage for the fast "+
				"class; the StatefulSet's claim is forbidden and pod-0 "+
				"stays Pending. Other tenants are unaffected.",
			"resourcequota/tenant-c/storage-caps", "notify",
			"statefulset/tenant-c/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			quota := &corev1.ResourceQuota{
				ObjectMeta: clusterMeta(c, "tenant-c", "storage-caps",
					"quota"),
				Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
					"fast.storageclass.storage.k8s.io/requests.storage": resource.
						MustParse("0")}}}
			c.list(quota)
			sts := g10Sts(c, "tenant-c", "db", nil, nil)
			c.after(5 * time.Minute)
			one := int32(1)
			sts.Spec.Replicas = &one
			c.update(sts)
			c.warn(c.warningEvent(sts, "StatefulSet", "FailedCreate",
				"create Claim data-db-0 Pod db-0 in StatefulSet db failed "+
					"error: persistentvolumeclaims \"data-db-0\" is "+
					"forbidden: exceeded quota: storage-caps, requested: "+
					"fast.storageclass.storage.k8s.io/requests.storage=10Gi, "+
					"used: 0, limited: 0", "statefulset-controller", 6))
		},
	}
}

// gap18TopologySpread: a DoNotSchedule spread over zones with full zone-c
// nodes leaves two of six replicas Pending.
func gap18TopologySpread() scenario {
	return scenario{
		expect: g2Exp("zone-spread-leaves-replicas-pending",
			"A Deployment spreads six replicas over zones with maxSkew 1 "+
				"DoNotSchedule; zone-c nodes are full, two replicas are "+
				"Pending on the spread constraint and four serve.",
			"scheduling//didn't match pod topology spread constraints",
			"notify").tail(10 * time.Minute),
		build: func(c *cluster) {
			g18Zones(c)
			place := []string{"n1", "n2", "n3", "n4", "n5", "n6"}
			w := g18Deploy(c, "shop", "api", place, func(spec *corev1.PodSpec) {
				spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{
					{MaxSkew: 1, TopologyKey: corev1.LabelTopologyZone,
						WhenUnsatisfiable: corev1.DoNotSchedule}}
			})
			c.after(5 * time.Minute)
			message := "0/6 nodes are available: 2 Insufficient cpu, 4 " +
				"node(s) didn't match pod topology spread constraints."
			for _, i := range []int{4, 5} {
				c.update(w.pod(i, "", pendingUnscheduled(message)))
			}
			g10Settle(c, w, 4)
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				pod := last(c, w.pod(4, ""))
				c.warn(c.warningEvent(pod, "Pod", "FailedScheduling",
					message, "default-scheduler", m))
			}
		},
	}
}

// gap18RuntimeClass: pool-b nodes lack the gvisor handler.
func gap18RuntimeClass() scenario {
	return scenario{
		expect: g2Exp("one-pool-runtimeclass-handler-missing",
			"A Deployment with runtimeClassName gvisor: pods on pool-a "+
				"run, pods that land on pool-b fail to create a sandbox "+
				"because no runsc handler is configured there.",
			"nodepool//pool-b", "notify", "deployment/secure/sandboxed").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(g18Node(c, "a1", "zone-a", "pool-a"),
				g18Node(c, "b1", "zone-a", "pool-b"),
				g18Node(c, "b2", "zone-a", "pool-b"))
			class := builtinObject(c, "node.k8s.io/v1", "RuntimeClass", "",
				"gvisor", map[string]any{}, map[string]any{})
			class.Object["handler"] = "runsc"
			c.list(class)
			place := []string{"a1", "b1", "b2"}
			w := g18Deploy(c, "secure", "sandboxed", place,
				func(spec *corev1.PodSpec) {
					name := c.n("gvisor")
					spec.RuntimeClassName = &name
				})
			c.after(5 * time.Minute)
			message := "Failed to create pod sandbox: no runtime for " +
				"\"runsc\" is configured"
			for _, i := range []int{1, 2} {
				pod := w.pod(i, place[i], startedNow, waiting(
					"ContainerCreating", ""))
				c.update(pod)
				g10Event(c, pod, "FailedCreatePodSandBox", message, "kubelet",
					4, c.now)
			}
			g10Settle(c, w, 1)
		},
	}
}

// gap18OrdinalCorrupt: one ordinal of a quorum of three crash-loops.
func gap18OrdinalCorrupt() scenario {
	return scenario{
		expect: g2Exp("statefulset-one-ordinal-data-corrupt",
			"db-2 of a three-replica StatefulSet crash-loops (exit 1) "+
				"after a reboot; db-0 and db-1 are Ready and quorum "+
				"holds.", "statefulset/data/db", "notify").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			sts := g10Sts(c, "data", "db", gap10Nodes, nil)
			for restarts := int32(2); restarts <= 6; restarts += 2 {
				c.after(3 * time.Minute)
				c.update(operatorBroker(c, sts, 2, "n3", crashLoop(1, "Error",
					"database files are corrupt", restarts)))
			}
		},
	}
}

// gap18UDPBlocked: a policy that admits TCP 53 only; UDP queries time out
// while every pod is Ready.
func gap18UDPBlocked() scenario {
	return scenario{
		expect: g2Exp("udp-only-blocked-by-policy",
			"A Service exposes TCP and UDP 53; the NetworkPolicy "+
				"allow-tcp-only admits TCP 53 only, so UDP queries time "+
				"out. All pods are Ready.", "networkpolicy/infra/"+
				"allow-tcp-only", "notify").budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"n1", "n2"}
			for _, name := range place {
				c.list(c.node(name, "zone-a"))
			}
			w := g18Deploy(c, "infra", "dns-proxy", place, nil)
			c.after(5 * time.Minute)
			tcp, port := corev1.ProtocolTCP, intstr.FromInt32(53)
			c.create(&networkingv1.NetworkPolicy{
				ObjectMeta: clusterMeta(c, "infra", "allow-tcp-only", "netpol"),
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{MatchLabels: w.
						deployment.Spec.Template.Labels},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress},
					Ingress: []networkingv1.NetworkPolicyIngressRule{{
						Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp,
							Port: &port}}}}}})
		},
	}
}

// gap18ReadinessGate: two of five pods have ready containers but an
// external controller never sets the gate condition.
func gap18ReadinessGate() scenario {
	gate := corev1.PodConditionType("lb.example/target-healthy")
	return scenario{
		expect: g2Exp("readiness-gate-unmet-on-some-pods",
			"Two of five pods stay Ready=False with every container "+
				"ready: the template's readiness gate is never set to "+
				"True by the external controller.", "deployment/shop/api",
			"notify").budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			place := []string{"n1", "n2", "n3", "n1", "n2"}
			for _, name := range gap10Nodes {
				c.list(c.node(name, "zone-a"))
			}
			w := g18Deploy(c, "shop", "api", place, func(spec *corev1.PodSpec) {
				spec.ReadinessGates = []corev1.PodReadinessGate{{
					ConditionType: gate}}
			})
			c.after(5 * time.Minute)
			unmet := func(c *cluster, pod *corev1.Pod) {
				for i := range pod.Status.Conditions {
					if pod.Status.Conditions[i].Type == corev1.PodReady {
						pod.Status.Conditions[i].Status = corev1.ConditionFalse
						pod.Status.Conditions[i].Reason = "ReadinessGatesNotReady"
						pod.Status.Conditions[i].LastTransitionTime = metav1.
							NewTime(c.now)
					}
				}
				pod.Status.Conditions = append(pod.Status.Conditions,
					corev1.PodCondition{Type: gate, Status: corev1.
						ConditionFalse})
			}
			g18Fail(c, w, place, []int{3, 4}, unmet)
		},
	}
}

// gap18LocalPolicy: externalTrafficPolicy Local leaves four of six nodes
// failing the load balancer health check by design.
func gap18LocalPolicy() scenario {
	return scenario{
		expect: g2Quiet("local-traffic-policy-unhealthy-nodes-quiet",
			"A Service with externalTrafficPolicy Local: the cloud load "+
				"balancer marks the four nodes without a pod unhealthy. "+
				"Designed behaviour; no warnings.").tail(10 * time.Minute),
		build: func(c *cluster) {
			for _, name := range []string{"n1", "n2", "n3", "n4", "n5", "n6"} {
				c.list(c.node(name, "zone-a"))
			}
			g18Deploy(c, "shop", "web", []string{"n1", "n2"}, nil)
			c.list(&corev1.Service{
				ObjectMeta: c.meta("shop", "web"),
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.
						ServiceExternalTrafficPolicyLocal,
					Selector: map[string]string{"app": c.n("web")}}})
			c.after(10 * time.Minute)
		},
	}
}
