package scenarios

import "time"

// gap24PlatformScenarios: node, storage, quota, admission, scheduling,
// registry and routing faults.
func gap24PlatformScenarios() []scenario {
	return []scenario{
		gap24QuotaForbids(), gap24RegistryRateLimited(),
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
