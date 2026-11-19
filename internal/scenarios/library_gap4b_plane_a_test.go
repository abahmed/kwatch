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

// gap4bPlaneAScenarios are changes to node agents, DNS, admission and the
// API server that reach many namespaces.
func gap4bPlaneAScenarios() []scenario {
	return []scenario{
		gap4bSelectorLabelRemoved(), gap4bCoreDNSOOM(),
		gap4bFinalizerPodsHoldIPs(), gap4bWebhookBackendEvicted(),
		gap4bAgentRolloutLoadsAPI(), gap4bCRDGrowth(), gap4bEtcdProbeLoop(),
	}
}

// gap4bSelectorLabelRemoved: a node upgrade drops the control-plane label
// the node agent DaemonSet selects; its desired count falls to zero, its
// pods are removed and new pods on those nodes get no network.
func gap4bSelectorLabelRemoved() scenario {
	return scenario{
		expect: g2Exp("daemonset-selector-matches-no-node-after-upgrade",
			"A node upgrade removes the label a networking DaemonSet's "+
				"nodeSelector requires; it selects no node, its pods "+
				"are gone and new pods cannot set up their network.",
			"daemonset/kube-system/net-agent", "notify", "node//n1",
			"node//n2", "deployment/shop/web", "deployment/billing/invoices").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			const label = "node-role.kubernetes.io/net"
			var nodes []*corev1.Node
			for _, name := range []string{"n1", "n2"} {
				node := c.node(name, "zone-a")
				node.Labels[label] = ""
				nodes = append(nodes, node)
				c.list(node)
			}
			ds := g2Daemon(c, "kube-system", "net-agent", "n1", "n2")
			ds.Spec.Template.Spec.NodeSelector = map[string]string{label: ""}
			c.update(ds)
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices"},
				[]string{"n1"})
			c.after(10 * time.Minute)
			for _, node := range nodes {
				upgraded := last(c, node)
				delete(upgraded.Labels, label)
				c.update(upgraded)
			}
			c.remove(agentPod(c, ds, "n1"), agentPod(c, ds, "n2"))
			setAgentCounts(ds, 0, 0)
			c.update(ds)
			message := "Failed to create pod sandbox: rpc error: code = " +
				"Unknown desc = failed to setup network for sandbox: " +
				"plugin type=\"net-agent\" failed (add): cni plugin not " +
				"initialized"
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
			}
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n2", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedCreatePodSandBox", message,
						round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap4bCoreDNSOOM: CoreDNS has a 70Mi memory limit sized from a quiet
// day; a retry-driven query spike OOM-kills both pods again and again and
// lookups fail in three namespaces.
func gap4bCoreDNSOOM() scenario {
	return scenario{
		expect: g2Exp("coredns-memory-limit-oom-loop",
			"Both CoreDNS pods are OOMKilled in a loop at a 70Mi limit "+
				"while query volume rises; lookups fail in three "+
				"namespaces and the DNS probe fails.",
			"deployment/kube-system/coredns", "page", "node//n1",
			"node//n2", "deployment/shop/cart",
			"deployment/billing/invoices").budget(3),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
			dns := c.deployment("kube-system", "coredns",
				"registry.k8s.io/coredns/coredns:v1.11.1", 2)
			configTemplate(dns, func(spec *corev1.PodSpec) {
				spec.Containers[0].Resources.Limits = corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("70Mi")}
			})
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:3.1", 2)
			invoices := c.deployment("billing", "invoices",
				"registry.example.com/invoices:1.8", 2)
			for _, w := range []*workload{dns, cart, invoices} {
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			}
			c.probe(kube.ClusterDNS, "")
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(dns.pod(0, "n1", crashLoop(137, "OOMKilled", "",
					restarts)), dns.pod(1, "n2", crashLoop(137, "OOMKilled",
					"", restarts)))
				dns.setReady(0)
				c.update(dns.objects())
				c.probe(kube.ClusterDNS, "lookup kubernetes.default.svc."+
					"cluster.local on 10.96.0.10:53: read udp 10.244.1."+
					"7:41234->10.96.0.10:53: i/o timeout")
				if restarts >= 2 {
					clusterDNSVictims(c, cart, "db.shop.svc.cluster.local",
						restarts-1)
					clusterDNSVictims(c, invoices,
						"payments-api.billing.svc.cluster.local",
						restarts-1)
				}
				c.after(30 * time.Second)
			}
		},
	}
}

// gap4bFinalizerPodsHoldIPs: an operator was removed and left eight pods
// Terminating on its finalizer; their addresses are still allocated and
// new pods of another namespace cannot get one.
func gap4bFinalizerPodsHoldIPs() scenario {
	finalizer := "jobs.example.com/cleanup"
	return scenario{
		expect: g2Exp("terminating-pods-hold-addresses",
			"Eight pods held by a finalizer whose operator is gone stay "+
				"Terminating for hours; the address pool is used up and "+
				"new pods elsewhere cannot get an address.",
			"finalizer/jobs/"+finalizer, "notify", "node//n1",
			"deployment/shop/web").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			runner := c.deployment("jobs", "runner",
				"registry.example.com/runner:2", 8)
			c.list(runner.objects())
			for i := range 8 {
				pod := runner.pod(i, "n1")
				deleting(&pod.ObjectMeta, c.now.Add(-3*time.Hour),
					finalizer)
				c.list(pod)
			}
			web := c.deployment("shop", "web", "registry.example.com/web:7", 1)
			c.list(web.objects())
			c.list(web.pod(0, "n1"))
			c.after(10 * time.Minute)
			*web.deployment.Spec.Replicas = 3
			web.setReady(1)
			c.update(web.objects())
			message := "Failed to create pod sandbox: rpc error: code = " +
				"Unknown desc = failed to setup network for sandbox: " +
				"failed to assign an IP address to container"
			for round := int32(1); round <= 5; round++ {
				for i := 1; i <= 2; i++ {
					pod := web.pod(i, "n1", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedCreatePodSandBox", message,
						round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap4bWebhookBackendEvicted: the only replica of a fail-closed webhook's
// backend is evicted for a higher-priority batch; no pod can be admitted,
// including the backend's own replacement.
func gap4bWebhookBackendEvicted() scenario {
	return scenario{
		expect: g2Exp("webhook-backend-evicted-cannot-return",
			"A fail-closed webhook's single backend pod is evicted; "+
				"every pod create is refused because the webhook has no "+
				"endpoint, including the backend's replacement.",
			"validatingwebhookconfiguration//policy-guard", "page",
			"node//n1", "deployment/shop/orders",
			"deployment/billing/ledger").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			guard := c.deployment("policy-system", "guard",
				"registry.example.com/guard:1", 1)
			c.list(guard.objects())
			c.list(guard.pod(0, "n1"))
			c.list(clusterService(c, "policy-system", "guard", 443),
				clusterSlice(c, "policy-system", "guard", guard.pod(0, "n1")))
			c.list(clusterValidatingHook(c, "policy-guard",
				"guard.policy.example.com", "policy-system", "guard"))
			fleet := gap2Spread(c, []string{"shop/orders", "billing/ledger"},
				[]string{"n1", "n2"})
			c.after(10 * time.Minute)
			c.update(guard.pod(0, "n1", evicted("The node was low on "+
				"resource: memory.")))
			guard.setReady(0)
			c.update(guard.objects())
			c.update(clusterSlice(c, "policy-system", "guard"))
			message := "Internal error occurred: failed calling webhook " +
				"\"guard.policy.example.com\": failed to call webhook: " +
				"Post \"https://guard.policy-system.svc:443/validate?" +
				"timeout=10s\": no endpoints available for service \"guard\""
			for _, w := range append(fleet, guard) {
				rs := w.rollout(func(spec *corev1.PodSpec) {
					spec.Containers[0].Env = []corev1.EnvVar{{
						Name: "REV", Value: "2"}}
				})
				d := w.deployment
				d.Status.UpdatedReplicas = 0
				d.Status.Conditions = []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentReplicaFailure, Status: "True",
					Reason: "FailedCreate", Message: message,
					LastTransitionTime: metav1.NewTime(c.now)}}
				c.update(d)
				c.create(rs)
			}
			for m := int32(1); m <= 5; m++ {
				for _, w := range append(fleet, guard) {
					g2Event(c, w.replicaSet, "ReplicaSet", "FailedCreate",
						message, "replicaset-controller", m)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap4bAgentRolloutLoadsAPI: a node inventory DaemonSet's new revision
// lists large objects from every node; the API server answers other
// clients with 429 during the rollout, and twenty minutes later cached DNS
// records expire and lookups fail.
func gap4bAgentRolloutLoadsAPI() scenario {
	return scenario{
		expect: g2Exp("daemonset-rollout-overloads-api-then-dns",
			"A DaemonSet rollout whose pods list large objects overloads "+
				"the API server (429 responses); twenty minutes later "+
				"lookups fail across namespaces.",
			"daemonset/kube-system/node-inventory", "page", "node//n1",
			"node//n2", "cluster-dns//cluster-dns", "apiserver//"+
				"kube-apiserver").budget(4).tail(15 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := controlPlaneFleet(c)
			ds := g2Daemon(c, "kube-system", "node-inventory", "n1", "n2")
			c.probe(kube.APIServer, "")
			c.probe(kube.ClusterDNS, "")
			c.after(10 * time.Minute)
			next := ds.DeepCopy()
			next.Generation, next.Status.ObservedGeneration = 2, 2
			next.Spec.Template.Spec.Containers[0].Image =
				"registry.example.com/node-inventory:2"
			c.update(next)
			c.update(agentPod(c, next, "n1", startedNow),
				agentPod(c, next, "n2", startedNow))
			message := "failed to list *v1.Pod: the server has received " +
				"too many requests and has asked us to try again later " +
				"(get pods)"
			for round := int32(1); round <= 6; round++ {
				c.probe(kube.APIServer, "Get \"https://10.96.0.1:443/"+
					"readyz\": the server has received too many "+
					"requests and has asked us to try again later")
				gap2CrashPods(c, fleet, []string{"n1", "n2"}, []int{0, 1},
					message, round)
				c.after(time.Minute)
			}
			c.after(20 * time.Minute)
			for round := int32(1); round <= 3; round++ {
				c.probe(kube.ClusterDNS, "lookup kubernetes.default.svc."+
					"cluster.local on 10.96.0.10:53: i/o timeout")
				clusterDNSVictims(c, fleet[0], "db.shop.svc.cluster.local",
					10+round)
				c.after(time.Minute)
			}
		},
	}
}

// gap4bCRDGrowth: a component stores a high-churn custom resource with no
// expiry; there are tens of thousands of them, the operator's list times
// out and it crash-loops.
func gap4bCRDGrowth() scenario {
	return scenario{
		expect: g2Exp("crd-instances-grow-unbounded-list-times-out",
			"An operator crash-loops because listing the instances of "+
				"a custom resource that grows without expiry times out "+
				"at the API server.",
			"customresourcedefinition//authrequests.auth.example",
			"notify", "deployment/auth/dex-operator", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			crd := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata": map[string]any{
					"name": "authrequests.auth.example"},
				"spec": map[string]any{"group": "auth.example"},
			}}
			crd.SetUID("crd-authrequests")
			crd.SetCreationTimestamp(metav1.NewTime(
				c.start.Add(-30 * 24 * time.Hour)))
			c.list(crd)
			op := c.deployment("auth", "dex-operator",
				"registry.example.com/dex-operator:2", 1)
			c.list(op.objects())
			c.list(op.pod(0, "n1"))
			c.after(5 * time.Minute)
			message := "failed to list *v1.AuthRequest: the server was " +
				"unable to return a response in the time allotted, but " +
				"may still be processing the request (get " +
				"authrequests.auth.example)"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(op.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)))
				op.setReady(0)
				c.update(op.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap4bEtcdProbeLoop: etcd's liveness probe times out under load and the
// kubelet restarts it about every two minutes; the cluster looks healthy
// for a short time after each restart.
func gap4bEtcdProbeLoop() scenario {
	return scenario{
		expect: g2Exp("etcd-restarted-by-own-liveness-probe",
			"etcd's liveness probe times out and the kubelet restarts it "+
				"every couple of minutes, with short healthy windows in "+
				"between.", "etcd//etcd", "page", "apiserver//"+
				"kube-apiserver", "node//n1").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			etcd := c.deployment("kube-system", "etcd",
				"registry.k8s.io/etcd:3.5.12", 1)
			c.list(etcd.objects())
			c.list(etcd.pod(0, "n1"))
			c.probe(kube.Etcd, "")
			c.probe(kube.APIServer, "")
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				pod := etcd.pod(0, "n1", crashLoop(2, "Error", "", 120+
					restarts))
				c.update(pod)
				gapKubelet(c, pod, "Unhealthy", "Liveness probe failed: "+
					"Get \"https://127.0.0.1:2379/health\": context "+
					"deadline exceeded", restarts)
				c.probe(kube.Etcd, "[-]etcd failed: reason withheld")
				c.after(40 * time.Second)
				c.probe(kube.Etcd, "")
				c.after(80 * time.Second)
			}
		},
	}
}
