package scenarios

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// advSameNodeIndependent: ledger is OOM-killed at its own limit and
// search crash-loops on a panic, both single replicas on n1, beside two
// healthy workloads on the same node. Two faults that share a node by
// chance; the node is healthy and neither is the other's cause.
func advSameNodeIndependent() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-same-node-independent-faults",
			Description: "Two unrelated workloads fail at the same time " +
				"on one node that also runs healthy workloads.",
			Root:        "deployment/billing/ledger",
			OtherRoots:  []string{"deployment/shop/search"},
			Tier:        "notify",
			MaxMessages: 4,
			MustNotBlame: []string{"node//n1", "zone//zone-a",
				"deployment/shop/cart", "deployment/shop/web"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			ledger := c.deployment("billing", "ledger",
				"registry.example.com/ledger:2", 1)
			search := c.deployment("shop", "search",
				"registry.example.com/search:7", 1)
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:5", 1)
			web := c.deployment("shop", "web", "registry.example.com/web:5", 1)
			for _, w := range []*workload{ledger, search, cart, web} {
				c.list(w.objects())
				c.list(w.pod(0, "n1"))
			}
			c.after(3 * time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(ledger.pod(0, "n1", crashLoop(137, "OOMKilled", "",
					restarts), longRun(40*time.Minute)))
				c.update(search.pod(0, "n1", crashLoop(2, "Error",
					"panic: unexpected end of JSON input", restarts)))
				ledger.setReady(0)
				search.setReady(0)
				c.update(ledger.objects())
				c.update(search.objects())
				c.after(time.Minute)
			}
		},
	}
}

func advClusterBinding(c *cluster, name string) *unstructured.Unstructured {
	meta := clusterMeta(c, "", name, "crb")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"subjects": []any{map[string]any{"kind": "ServiceAccount",
			"name": "controller", "namespace": c.n("shop")}},
		"roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io",
			"kind": "ClusterRole", "name": "pod-reader"},
	}}
	u.SetName(meta.Name)
	u.SetUID(meta.UID)
	return u
}

// advClusterRoleBindingDeleted: the ClusterRoleBinding that lets the
// controller list pods across the cluster is pruned; the controller's
// log says it may not list pods at the cluster scope.
func advClusterRoleBindingDeleted() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-clusterrolebinding-deleted",
			Description: "A ClusterRoleBinding is deleted; the pods of the " +
				"ServiceAccount it bound crash with a forbidden error " +
				"at the cluster scope.",
			Root: "clusterrolebinding//ops-pod-reader", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/controller", "node//n1",
				"node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			ctl := c.deployment("shop", "controller",
				"registry.example.com/controller:1.3", 2)
			c.list(ctl.objects())
			c.list(ctl.pod(0, "n1"), ctl.pod(1, "n2"))
			binding := advClusterBinding(c, "ops-pod-reader")
			c.list(binding)
			c.after(time.Minute)
			c.remove(binding)
			c.after(2 * time.Minute)
			line := "level=fatal msg=\"pods is forbidden: User " +
				"\\\"system:serviceaccount:" + c.n("shop") + ":controller" +
				"\\\" cannot list resource \\\"pods\\\" in API group " +
				"\\\"\\\" at the cluster scope\""
			advLogCrash(c, ctl, line, 5)
		},
	}
}

// advTwoRegistriesIndependent: one workload cannot pull from a private
// registry (401) and another cannot pull from a public registry (429
// rate limit), at the same time. Two registries, two incidents.
func advTwoRegistriesIndependent() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-two-registries-independent",
			Description: "Pulls fail with 401 on a private registry and " +
				"with 429 on a public one at the same time.",
			Root:        "registry//registry.corp.example",
			OtherRoots:  []string{"registry//registry-1.docker.io"},
			Tier:        "notify",
			MaxMessages: 4,
			MustNotBlame: []string{"node//n1", "node//n2",
				"deployment/shop/web", "deployment/batch/job"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			web := c.deployment("shop", "web",
				clusterRegistry+"/shop/web:2.0", 2)
			job := c.deployment("batch", "job",
				"registry-1.docker.io/library/busybox:1.36", 2)
			for _, w := range []*workload{web, job} {
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			}
			c.after(3 * time.Minute)
			setReplicas(web, 4)
			setReplicas(job, 4)
			web.setReady(2)
			job.setReady(2)
			c.update(web.objects())
			c.update(job.objects())
			auth := "failed to authorize: failed to fetch anonymous " +
				"token: unexpected status from GET request to https://" +
				clusterRegistry + "/v2/token: 401 Unauthorized"
			limit := "unexpected status from HEAD request to https://" +
				"registry-1.docker.io/v2/library/busybox/manifests/1.36: " +
				"429 Too Many Requests: toomanyrequests: You have reached " +
				"your pull rate limit"
			for attempt := range 6 {
				for i, node := range []string{"n1", "n2"} {
					clusterPullFailure(c, web, i+2, node, attempt, auth)
					clusterPullFailure(c, job, i+2, node, attempt, limit)
				}
				c.after(40 * time.Second)
			}
		},
	}
}
