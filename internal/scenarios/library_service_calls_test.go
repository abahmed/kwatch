package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// serviceCallScenarios are workloads failing on a Service of the same
// cluster that their error text names. The Service and its pods are
// ordinary objects kwatch watches; the error line only links them.
func serviceCallScenarios() []scenario {
	return []scenario{redisDownLinkedByError(), cacheScaledToZero(),
		policyBlocksDependency()}
}

// redisError is what the API prints when its cache refuses connections.
const redisError = "dial tcp redis:6379: connect: connection refused"

// redisDownLinkedByError: the shop cache loses its volume and its only
// pod crash-loops, so Service redis has no ready endpoints. The two API
// replicas that call it by name crash with a refused connection. The
// cache is the root, and the API belongs to its incident.
func redisDownLinkedByError() scenario {
	return scenario{
		expect: expectation{
			Name: "redis-down-linked-by-error",
			Description: "A Service loses every ready endpoint and the " +
				"API crashes with an error that names the Service.",
			Root: "deployment/shop/redis", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cache := c.deployment("shop", "redis",
				"registry.example.com/redis:7", 1)
			c.list(cache.objects())
			cachePod := cache.pod(0, "n1")
			c.list(cachePod, clusterService(c, "shop", "redis", 6379),
				trafficSlice(c, "redis", cachePod))
			api := serviceCallApp(c)
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(cache.pod(0, "n1", crashLoop(1, "Error",
					"FATAL: cannot open the append only file: "+
						"No space left on device", restarts)))
				cache.setReady(0)
				c.update(cache.objects())
				c.update(trafficUnreadySlice(c, "redis", cache.pod(0, "n1")))
				for replica := range 2 {
					c.update(api.pod(replica, "n1", crashLoop(1, "Error",
						redisError, restarts)))
				}
				api.setReady(0)
				c.update(api.objects())
				c.after(time.Minute)
			}
		},
	}
}

// serviceCallApp lists the shop API, whose containers read the cache
// address from REDIS_ADDR.
func serviceCallApp(c *cluster) *workload {
	api := c.deployment("shop", "api", "registry.example.com/api:5", 2)
	configTemplate(api, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "REDIS_ADDR", Value: c.n("redis") + ":6379",
		}}
	})
	c.list(api.objects())
	c.list(api.pod(0, "n1"), api.pod(1, "n1"))
	return api
}

// cacheScaledToZero: the shop storefront's web pods call Service cache
// by name and crash with a refused connection after someone scales the
// cache to zero. An Ingress routes to web. The scaled Deployment is the
// root; the Service only has no endpoints, and the web pods are what
// users reach through the Ingress.
func cacheScaledToZero() scenario {
	return scenario{
		expect: expectation{
			Name: "ingress-service-no-endpoints-chain",
			Description: "An Ingress routes to a web Service whose pods " +
				"crash calling a cache Service that was scaled to zero.",
			Root: "deployment/shop/cache", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/web", "node//n1",
				"service/shop/cache"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cache := c.deployment("shop", "cache",
				"registry.example.com/cache:2", 1)
			c.list(cache.objects())
			cachePod := cache.pod(0, "n1")
			c.list(cachePod, clusterService(c, "shop", "cache", 6379),
				trafficSlice(c, "cache", cachePod))
			web := c.deployment("shop", "web", "registry.example.com/web:9", 2)
			configTemplate(web, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{{
					Name: "CACHE_URL", Value: "redis://" + c.n("cache"),
				}}
			})
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n1"))
			c.list(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", web.pod(0, "n1"), web.pod(1, "n1")))
			c.list(trafficIngress(c, "web", ""))
			c.after(10 * time.Minute)
			scaleToZero(c, cache)
			c.remove(cachePod)
			c.update(trafficSlice(c, "cache"))
			c.after(30 * time.Second)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for replica := range 2 {
					c.update(web.pod(replica, "n1", crashLoop(1, "Error",
						"redis: dial tcp "+c.n("cache")+
							":6379: connect: connection refused", restarts)))
				}
				web.setReady(0)
				c.update(web.objects())
				c.after(time.Minute)
			}
		},
	}
}

// scaleToZero records kubectl scaling the workload to no replicas.
func scaleToZero(c *cluster, w *workload) {
	zero := int32(0)
	scaled := w.deployment.DeepCopy()
	scaled.Spec.Replicas = &zero
	scaled.Status.Replicas, scaled.Status.ReadyReplicas = 0, 0
	scaled.Status.AvailableReplicas, scaled.Status.UpdatedReplicas = 0, 0
	scaled.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager: "kubectl-scale", Operation: metav1.ManagedFieldsOperationUpdate,
		Time: &metav1.Time{Time: c.now},
	}}
	w.deployment = scaled
	c.update(scaled)
}

// policyBlocksDependency: a default-deny NetworkPolicy created in shop
// selects every pod. The orders API, which is configured to call Service
// postgres, times out on its database; its pods are healthy otherwise,
// and the database pods stay ready. The policy is the root, and the
// message says which call it blocks.
func policyBlocksDependency() scenario {
	return scenario{
		expect: expectation{
			Name: "networkpolicy-blocks-dependency",
			Description: "A default-deny NetworkPolicy is created and the " +
				"API times out calling the Service its environment names.",
			Root: "networkpolicy/shop/deny-all", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"deployment/shop/postgres", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			db := c.deployment("shop", "postgres",
				"registry.example.com/postgres:16", 1)
			c.list(db.objects())
			dbPod := db.pod(0, "n1")
			c.list(dbPod, clusterService(c, "shop", "postgres", 5432),
				trafficSlice(c, "postgres", dbPod))
			api := c.deployment("shop", "orders",
				"registry.example.com/orders:4", 2)
			configTemplate(api, func(spec *corev1.PodSpec) {
				spec.Containers[0].Env = []corev1.EnvVar{{
					Name: "DATABASE_URL", Value: "postgres://orders@" +
						c.n("postgres") + ":5432/orders",
				}}
			})
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n1"))
			c.after(5 * time.Minute)
			c.create(denyAllPolicy(c))
			c.after(20 * time.Second)
			message := "FATAL: could not connect to server: Connection " +
				"timed out; dial tcp 10.96.14.3:5432: i/o timeout"
			for restarts := int32(1); restarts <= 5; restarts++ {
				for replica := range 2 {
					c.update(api.pod(replica, "n1", crashLoop(1, "Error",
						message, restarts)))
				}
				api.setReady(0)
				c.update(api.objects())
				c.after(45 * time.Second)
			}
		},
	}
}

// denyAllPolicy is the shop's default-deny policy: it selects every pod
// and allows nothing in either direction.
func denyAllPolicy(c *cluster) *networkingv1.NetworkPolicy {
	meta := clusterMeta(c, "shop", "deny-all", "netpol")
	meta.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager: "bob", Operation: metav1.ManagedFieldsOperationUpdate,
		Time: &metav1.Time{Time: c.now},
	}}
	return &networkingv1.NetworkPolicy{
		ObjectMeta: meta,
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress,
			},
		},
	}
}
