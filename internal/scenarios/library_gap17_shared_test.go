package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// gap17SharedScenarios: a shared backing service or entry point fails and
// workloads of other namespaces fail with it.
func gap17SharedScenarios() []scenario {
	return []scenario{
		gap17PostgresDown(), gap17CacheScaledToZero(),
		gap17IngressControllerDown(), gap17NFSDown(), gap17EgressProxyDown(),
		gap17AuthJWKSUnreachable(), gap17CacheRollingQuiet(),
		gap17DatabaseFailoverDigest(),
	}
}

func gap17PostgresDown() scenario {
	return scenario{
		expect: g2Exp("shared-postgres-down-three-namespaces",
			"A shared StatefulSet database in its own namespace is "+
				"OOMKilled and crash-loops; Deployments of three other "+
				"namespaces log its cross-namespace name with no "+
				"endpoints. One incident, rooted at the database.",
			"statefulset/data/pg", "notify", "deployment/shop/api",
			"deployment/billing/ledger", "deployment/reports/gen").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			sts := g17Sts(c, "data", "pg", "n1")
			apps := g17Apps(c, 3, "shop/api", "billing/ledger",
				"reports/gen")
			c.after(10 * time.Minute)
			g17Loop(c, 6, func(round int32) {
				c.update(operatorBroker(c, sts, 0, "n1", crashLoop(137,
					"OOMKilled", "", round)))
				c.update(clusterSlice(c, "data", "pg"))
				g17Victims(c, apps, round, "dial tcp "+
					g17FQDN(c, "pg", "data")+":5432: connect: "+
					"connection refused")
			})
		},
	}
}

func gap17CacheScaledToZero() scenario {
	return scenario{
		expect: g2Exp("shared-cache-no-endpoints-cross-namespace",
			"A shared cache Deployment is scaled to zero by a values "+
				"edit; web pods of two other namespaces time out on its "+
				"Service and go NotReady. The edit is named.",
			"unknown", "notify",
			"deployment/shop/web", "deployment/jobs/render").
			budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cache := g17Up(c, "platform", "memcached", 2)
			apps := g17Apps(c, 2, "shop/web", "jobs/render")
			c.after(10 * time.Minute)
			scaleToZero(c, cache)
			d := cache.deployment
			d.ManagedFields[0].Manager = "helm"
			c.update(d)
			c.remove(cache.pod(0, "n1"), cache.pod(1, "n1"))
			c.update(clusterSlice(c, "platform", "memcached"))
			c.after(3 * time.Minute)
			g17Loop(c, 6, func(round int32) {
				for _, app := range apps {
					g17Unready(c, app, round, "i/o timeout "+
						g17FQDN(c, "memcached", "platform")+":11211")
				}
			})
		},
	}
}

func gap17IngressControllerDown() scenario {
	return scenario{
		expect: g2Exp("ingress-controller-down-four-namespaces",
			"The shared ingress controller crash-loops after a node "+
				"upgrade while every backend is Ready; the six "+
				"Ingresses of four namespaces are victims, not "+
				"separate incidents.",
			"deployment/ingress-nginx/controller", "page",
			"ingress/shop/storefront", "ingress/shop/admin",
			"ingress/billing/portal", "ingress/reports/site",
			"ingress/reports/api", "ingress/jobs/ui").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			ctl := g17UpService(c, "ingress-nginx", "controller", 2,
				corev1.ServiceTypeLoadBalancer)
			g17Apps(c, 2, "shop/web", "billing/web", "reports/web",
				"jobs/web")
			for _, ing := range [][2]string{{"shop", "storefront"},
				{"shop", "admin"}, {"billing", "portal"},
				{"reports", "site"}, {"reports", "api"},
				{"jobs", "ui"}} {
				c.list(g17Ingress(c, ing[0], ing[1], "web"))
			}
			c.after(10 * time.Minute)
			g17Loop(c, 6, func(round int32) {
				g17Crash(c, ctl, round, "failed to bind :443")
				c.update(clusterSlice(c, "ingress-nginx", "controller"))
			})
		},
	}
}

func gap17NFSDown() scenario {
	return scenario{
		expect: g2Exp("shared-nfs-server-down-mount-timeouts",
			"A shared NFS server Deployment is NotReady and Pending "+
				"after a drain; pods of three namespaces that mount its "+
				"volumes stay in ContainerCreating on mount timeouts.",
			"deployment/storage/nfs-server", "notify",
			"deployment/shop/web", "deployment/billing/api",
			"deployment/reports/gen").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			nfs := g17Up(c, "storage", "nfs-server", 1)
			c.list(scheduleClass(c, "nfs", "Immediate"))
			var apps []*workload
			for _, app := range [][2]string{{"shop", "web"},
				{"billing", "api"}, {"reports", "gen"}} {
				apps = append(apps, gap5bNFSConsumer(c, app[0], app[1]))
			}
			c.after(10 * time.Minute)
			c.update(nfs.pod(0, "", pendingUnscheduled("0/1 nodes are "+
				"available: 1 Insufficient memory.")))
			nfs.setReady(0)
			c.update(nfs.objects())
			c.update(clusterSlice(c, "storage", "nfs-server"))
			g17Loop(c, 6, func(round int32) {
				for _, app := range apps {
					g17Mount(c, app, round, "mount.nfs: Connection "+
						"timed out")
				}
			})
		},
	}
}

func gap17EgressProxyDown() scenario {
	return scenario{
		expect: g2Exp("shared-egress-proxy-down",
			"The shared egress proxy crash-loops after a config push; "+
				"pods of three namespaces with HTTPS_PROXY pointing at "+
				"it crash on every outbound call.",
			"deployment/egress/proxy", "notify", "deployment/shop/api",
			"deployment/shop/web", "deployment/billing/api",
			"deployment/billing/sync", "deployment/reports/gen",
			"deployment/reports/mail").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			proxy := g17Up(c, "egress", "proxy", 2)
			apps := g17Apps(c, 2, "shop/api", "shop/web", "billing/api",
				"billing/sync", "reports/gen", "reports/mail")
			c.after(10 * time.Minute)
			g17Loop(c, 6, func(round int32) {
				g17Crash(c, proxy, round, "invalid acl")
				c.update(clusterSlice(c, "egress", "proxy"))
				g17Victims(c, apps, round, "proxyconnect tcp: dial tcp "+
					g17FQDN(c, "proxy", "egress")+":3128: connect: "+
					"connection refused")
			})
		},
	}
}

func gap17AuthJWKSUnreachable() scenario {
	return scenario{
		expect: g2Exp("shared-auth-service-jwks-unreachable",
			"The shared auth service is not ready after a migration; "+
				"apps of three namespaces fetch its JWKS at start and "+
				"never become Ready, without restarts.",
			"deployment/identity/auth", "notify", "deployment/shop/api",
			"deployment/billing/api", "deployment/jobs/runner").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			auth := g17Up(c, "identity", "auth", 2)
			apps := g17Apps(c, 2, "shop/api", "billing/api", "jobs/runner")
			c.after(10 * time.Minute)
			g17Unready(c, auth, 1, "Readiness probe failed: HTTP probe "+
				"failed with statuscode: 503")
			c.update(clusterSlice(c, "identity", "auth"))
			c.after(30 * time.Minute)
			g17Loop(c, 8, func(round int32) {
				for _, app := range apps {
					g17Unready(c, app, round, "failed to fetch jwks "+
						"from http://"+g17FQDN(c, "auth", "identity")+
						"/.well-known/jwks.json: 503")
				}
			})
		},
	}
}

func gap17CacheRollingQuiet() scenario {
	return scenario{
		expect: g2Quiet("shared-cache-rolling-restart-brief-errors-quiet",
			"A shared cache rolls with one pod at a time; apps of three "+
				"namespaces log a refused connection once and restart "+
				"at most once, and its Service always has an endpoint."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cache := g17Up(c, "platform", "redis", 2)
			apps := g17Apps(c, 2, "shop/web", "billing/api", "jobs/runner")
			c.after(10 * time.Minute)
			rs := cache.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/redis:2"
			})
			c.update(cache.deployment)
			c.create(rs)
			c.update(cache.pod(0, "n1", startedNow, notReady))
			for _, app := range apps {
				c.update(app.pod(0, "n1", crashLoop(1, "Error",
					"dial tcp: connection refused", 1)))
			}
			c.after(40 * time.Second)
			cache.setReady(2)
			c.update(cache.objects())
			c.update(cache.pod(0, "n1"))
			for _, app := range apps {
				c.update(app.pod(0, "n1"))
			}
			c.after(5 * time.Minute)
		},
	}
}

func gap17DatabaseFailoverDigest() scenario {
	return scenario{
		expect: g2Quiet("shared-database-failover-digest",
			"A shared database fails over to a replica in under a "+
				"minute; two apps log three failed connects and restart "+
				"once, on the same days each month. Each failover is a "+
				"brief blip that recovers by itself.").
			tail(30 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			sts := g17Sts(c, "data", "pg", "n1")
			apps := g17Apps(c, 2, "shop/api", "billing/ledger")
			for day := 0; day < 3; day++ {
				c.after(7 * 24 * time.Hour)
				c.update(clusterSlice(c, "data", "pg"))
				for _, app := range apps {
					c.update(app.pod(0, "n1", crashLoop(1, "Error",
						"dial tcp: lookup "+g17FQDN(c, "pg", "data")+
							": no endpoints", 1)))
				}
				c.after(28 * time.Second)
				c.update(operatorBroker(c, sts, 0, "n1"))
				c.update(clusterSlice(c, "data", "pg",
					operatorBroker(c, sts, 0, "n1")))
				for _, app := range apps {
					c.update(app.pod(0, "n1"))
				}
			}
		},
	}
}
