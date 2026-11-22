package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// gap24PlatformScenarios: node, storage, quota, admission, scheduling,
// registry and routing faults.
func gap24PlatformScenarios() []scenario {
	return []scenario{
		gap24NodeLostDatabase(), gap24PVCFull(), gap24QuotaForbids(),
		gap24WebhookFailClosed(), gap24UnschedulableTaint(),
		gap24RegistryRateLimited(), gap24IngressBackendMissing(),
	}
}

func gap24NodeLostDatabase() scenario {
	return scenario{
		expect: g24Exp("wording-node-lost-pinned-database",
			"A node stops reporting with a database pod on it; the "+
				"message names the node and a node-level action, not "+
				"the pod.",
			"node//worker-3", "page", 2, "statefulset/data/pg"),
		build: func(c *cluster) {
			node := c.node("worker-3", "zone-a")
			c.list(c.node("worker-1", "zone-a"), node)
			sts := g17Sts(c, "data", "pg", "worker-3")
			c.after(10 * time.Minute)
			nodeLose(c, node)
			c.after(time.Minute)
			sts.Status.ReadyReplicas = 0
			c.update(sts)
			c.update(operatorBroker(c, sts, 0, "worker-3", notReady,
				nodeTaintEvicted))
			c.after(10 * time.Minute)
		},
	}
}

func gap24PVCFull() scenario {
	return scenario{
		expect: g24Exp("wording-pvc-full",
			"A 10Gi claim is full; the database crash-loops with no "+
				"space left. The message names the claim and its size "+
				"and guesses nothing about what filled it.",
			"persistentvolumeclaim/data/pg-data", "page", 2, "node//n1",
			"deployment/data/pg"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			class := scheduleClass(c, "standard-rwo", "WaitForFirstConsumer")
			claim := scheduleClaim(c, "data", "pg-data", "standard-rwo")
			size := resource.MustParse("10Gi")
			claim.Spec.Resources.Requests[corev1.ResourceStorage] = size
			claim.Spec.VolumeName = "pvc-" + string(claim.UID)
			claim.Status.Phase = corev1.ClaimBound
			claim.Status.Capacity = claim.Spec.Resources.Requests
			w := c.deployment("data", "pg", "registry.example.com/pg:16", 1)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{{Name: "data",
					VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.
						PersistentVolumeClaimVolumeSource{
						ClaimName: claim.Name}}}}
			})
			c.list(class, claim)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(5 * time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", "PANIC: "+
					"could not write to file \"pg_wal/xlogtemp.31\": No "+
					"space left on device", restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

func gap24QuotaForbids() scenario {
	return scenario{
		expect: g24Exp("wording-quota-forbids-pods",
			"A scale-out from three to five is refused because the "+
				"namespace's CPU request quota is used up; the message "+
				"names the quota, not the cluster.",
			"resourcequota/shop/compute", "notify", 1, "node//n1",
			"node//n2", "deployment/shop/api"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 3)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n1"))
			quota := clusterQuota(c, "8", "8")
			quota.ObjectMeta = clusterMeta(c, "shop", "compute", "quota")
			c.list(quota)
			c.after(5 * time.Minute)
			setReplicas(w, 5)
			w.setReady(3)
			c.update(w.objects())
			for n := int32(1); n <= 5; n++ {
				c.warn(c.warningEvent(w.replicaSet, "ReplicaSet",
					"FailedCreate", "Error creating: pods \"api-x\" is "+
						"forbidden: exceeded quota: "+c.n("compute")+
						", requested: requests.cpu=500m, used: "+
						"requests.cpu=8, limited: requests.cpu=8",
					"replicaset-controller", n*3))
				c.after(time.Minute)
			}
		},
	}
}

func gap24WebhookFailClosed() scenario {
	return scenario{
		expect: g24Exp("wording-webhook-down-fail-closed",
			"A fail-closed validating webhook's Service has no ready "+
				"pods; creates in two namespaces are refused. The "+
				"Service is the root and the message states the "+
				"effect on deploys.",
			"service/policy/policy-svc", "page", 2,
			"validatingwebhookconfiguration//policy-check",
			"deployment/shop/api", "deployment/billing/api"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			policy := g17Up(c, "policy", "policy-svc", 2)
			c.list(clusterValidatingHook(c, "policy-check",
				"check.policy.example.com", "policy", "policy-svc"))
			apps := g17Apps(c, 2, "shop/api", "billing/api")
			c.after(time.Minute)
			setReplicas(policy, 0)
			policy.setReady(0)
			c.update(policy.objects())
			c.remove(policy.pod(0, "n1"), policy.pod(1, "n1"))
			c.update(clusterSlice(c, "policy", "policy-svc"))
			c.after(30 * time.Second)
			g17FailedCreate(c, apps, "Internal error occurred: failed "+
				"calling webhook \"check.policy.example.com\": no "+
				"endpoints available for service \""+c.n("policy-svc")+"\"")
		},
	}
}

func gap24UnschedulableTaint() scenario {
	return scenario{
		expect: g24Exp("wording-unschedulable-taint",
			"A pod is Pending six minutes because all five nodes carry "+
				"a taint it does not tolerate; the message names the "+
				"exact taint and one of its two fixes.",
			"deployment/jobs/batch", "notify", 1),
		build: func(c *cluster) {
			for _, name := range []string{"t1", "t2", "t3", "t4", "t5"} {
				node := c.node(name, "zone-a")
				node.Spec.Taints = []corev1.Taint{{Key: "dedicated",
					Value: "gpu", Effect: "NoSchedule"}}
				c.list(node)
			}
			w := c.deployment("jobs", "batch", "registry.example.com/batch:2", 1)
			w.rollout(requestCPU("500m"))
			w.setReady(0)
			c.create(w.objects())
			message := "0/5 nodes are available: 5 node(s) had " +
				"untolerated taint {dedicated: gpu}. preemption: 0/5 " +
				"nodes are available: 5 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 6)
		},
	}
}

func gap24RegistryRateLimited() scenario {
	return scenario{
		expect: g24Exp("wording-registry-rate-limited",
			"A new node pool's pulls from docker.io are all answered "+
				"with a pull rate limit; six pods of two namespaces "+
				"fail. One registry is the root.",
			"registry//docker.io", "notify", 1, "node//n3"),
		build: func(c *cluster) {
			var apps []*workload
			for _, app := range [][2]string{{"shop", "api"}, {"shop", "web"},
				{"shop", "cart"}, {"billing", "api"}, {"billing", "ledger"},
				{"billing", "sync"}} {
				apps = append(apps, c.deployment(app[0], app[1],
					"docker.io/"+app[0]+"/"+app[1]+":4.2", 2))
			}
			loginOutage(c, apps, rateLimitAnswer)
		},
	}
}

func gap24IngressBackendMissing() scenario {
	return scenario{
		expect: g24Exp("wording-ingress-backend-missing",
			"An Ingress still routes to a Service that was renamed; "+
				"the message names the missing Service and the new "+
				"name, and guesses nothing about user-visible errors.",
			"ingress/shop/shop", "notify", 1, "service/shop/web",
			"deployment/shop/web"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "frontend", 8080),
				trafficSlice(c, "frontend", pods...))
			c.list(trafficIngress(c, "frontend", ""))
			c.after(10 * time.Minute)
			c.remove(clusterService(c, "shop", "frontend", 8080),
				trafficSlice(c, "frontend", pods...))
			c.create(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			c.after(5 * time.Minute)
		},
	}
}
