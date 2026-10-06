package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// chainScenarios are failures that travel more than one step: a cause
// that breaks a service, which breaks a second one that calls it, which
// empties what users reach.
func chainScenarios() []scenario {
	return []scenario{postgresChainToIngress(), chainWrongOrder(),
		chainCappedAtThree()}
}

// chainProbe is what a readiness probe of the API prints while its
// database is down.
const chainProbe = "Readiness probe failed: dial tcp postgres:5432: " +
	"connect: connection refused"

// chainTier is one workload of a chain with its Service.
type chainTier struct {
	w    *workload
	pods []*corev1.Pod
}

// chainTierOf lists a workload with one pod per replica, its Service
// and a ready EndpointSlice. callee, when set, is the Service its
// containers read from the DATABASE_URL variable.
func chainTierOf(c *cluster, name string, replicas int32, callee string,
) *chainTier {
	w := c.deployment("shop", name, "registry.example.com/"+name+":3",
		replicas)
	if callee != "" {
		configTemplate(w, func(spec *corev1.PodSpec) {
			spec.Containers[0].Env = []corev1.EnvVar{{
				Name: "DATABASE_URL", Value: "tcp://" + c.n(callee) + ":5432",
			}}
		})
	}
	c.list(w.objects())
	tier := &chainTier{w: w}
	for i := range int(replicas) {
		tier.pods = append(tier.pods, w.pod(i, "n1"))
	}
	c.list(clusterObjects(tier.pods)...)
	c.list(clusterService(c, "shop", name, 8080),
		trafficSlice(c, name, tier.pods...))
	return tier
}

// clusterObjects widens pods to the objects c.list takes.
func clusterObjects(pods []*corev1.Pod) []runtime.Object {
	out := make([]runtime.Object, 0, len(pods))
	for _, p := range pods {
		out = append(out, p)
	}
	return out
}

// postgresChainToIngress: the only postgres pod is OOM-killed at 10:00,
// the API's readiness probe fails on its database at 10:01 and the
// Service behind the Ingress has no ready endpoint at 10:02. One
// incident, rooted at postgres.
func postgresChainToIngress() scenario {
	return scenario{
		expect: expectation{
			Name: "chain-postgres-api-ingress",
			Description: "A database is OOM-killed, the API behind the " +
				"Ingress stops being ready because it cannot reach it.",
			Root: "deployment/shop/postgres", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			db := chainTierOf(c, "postgres", 1, "")
			api := chainTierOf(c, "api", 2, "postgres")
			c.list(trafficIngress(c, "api", ""))
			c.after(9*time.Minute + 55*time.Second)
			c.update(db.w.pod(0, "n1", crashLoop(137, "OOMKilled", "", 1)))
			db.w.setReady(0)
			c.update(db.w.objects())
			c.update(trafficUnreadySlice(c, "postgres", db.pods...))
			c.after(10 * time.Second)
			var pods []*corev1.Pod
			for i := range 2 {
				p := api.w.pod(i, "n1", notReady)
				pods = append(pods, p)
				c.update(p)
			}
			api.w.setReady(0)
			c.update(api.w.objects())
			c.update(trafficUnreadySlice(c, "api", pods...))
			for count := int32(1); count <= 6; count++ {
				for _, p := range pods {
					c.warn(c.warningEvent(p, "Pod", "Unhealthy", chainProbe,
						"kubelet", count))
				}
				c.after(time.Minute)
			}
		},
	}
}

// chainWrongOrder: the API crash-loops at 10:10 on a panic of its own,
// and postgres, which it calls by configuration, runs out of memory at
// 10:20. The API began first, so postgres cannot have caused it: two
// incidents, each with its own root.
func chainWrongOrder() scenario {
	return scenario{
		expect: expectation{
			Name: "chain-wrong-time-order",
			Description: "An API that calls postgres crash-loops ten " +
				"minutes before postgres runs out of memory.",
			Root: "deployment/shop/api", Tier: "page", MaxMessages: 5,
			OtherRoots: []string{"deployment/shop/postgres"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			db := chainTierOf(c, "postgres", 1, "")
			api := chainTierOf(c, "api", 2, "postgres")
			c.list(trafficIngress(c, "api", ""))
			c.after(10 * time.Minute)
			chainCrash(c, api, "panic: runtime error: index out of range",
				3)
			c.after(10 * time.Minute)
			c.update(db.w.pod(0, "n1", crashLoop(137, "OOMKilled", "", 1)))
			db.w.setReady(0)
			c.update(db.w.objects())
			c.update(trafficUnreadySlice(c, "postgres", db.pods...))
			c.after(5 * time.Minute)
		},
	}
}

// chainCrash crash-loops every pod of the tier with the message once a
// minute, for minutes minutes.
func chainCrash(c *cluster, tier *chainTier, message string, minutes int) {
	for restarts := int32(1); restarts <= int32(minutes); restarts++ {
		crashTier(c, tier, message, restarts)
		c.after(time.Minute)
	}
}

// unreadyTier makes every pod of the tier not ready and empties its
// Service, as a failing readiness probe does.
func unreadyTier(c *cluster, tier *chainTier) {
	name := tier.w.deployment.Name
	service := name[:len(name)-len(c.suffix)]
	var pods []*corev1.Pod
	for i := range tier.pods {
		pods = append(pods, tier.w.pod(i, "n1", notReady))
		c.update(pods[i])
	}
	tier.w.setReady(0)
	c.update(tier.w.objects())
	c.update(trafficUnreadySlice(c, service, pods...))
}

// crashTier crash-loops every pod of the tier and empties its Service.
func crashTier(c *cluster, tier *chainTier, message string, restarts int32) {
	name := tier.w.deployment.Name
	service := name[:len(name)-len(c.suffix)]
	var pods []*corev1.Pod
	for i := range tier.pods {
		p := tier.w.pod(i, "n1", crashLoop(1, "Error", message, restarts))
		pods = append(pods, p)
		c.update(p)
	}
	tier.w.setReady(0)
	c.update(tier.w.objects())
	c.update(trafficUnreadySlice(c, service, pods...))
}

// chainCappedAtThree: five workloads in a row, each calling the one
// before it. The first runs out of memory and the next four fail ten
// seconds apart. A chain follows three steps from the cause: the last
// workload fails past it, and its Service, which has no endpoints because
// of the third, is told as an incident of its own.
func chainCappedAtThree() scenario {
	return scenario{
		expect: expectation{
			Name: "chain-capped-at-three-steps",
			Description: "Five workloads that call each other in a row " +
				"fail one after another, from a database.",
			Root: "deployment/shop/postgres", Tier: "notify", MaxMessages: 6,
			OtherRoots: []string{"service/shop/web"},
			MustNotBlame: []string{"deployment/shop/api",
				"deployment/shop/gateway", "deployment/shop/web"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			db := chainTierOf(c, "postgres", 1, "")
			names := []string{"api", "gateway", "web", "edge"}
			tiers := make([]*chainTier, 0, len(names))
			callee := "postgres"
			for _, name := range names {
				tiers = append(tiers, chainTierOf(c, name, 2, callee))
				callee = name
			}
			c.after(9*time.Minute + 55*time.Second)
			c.update(db.w.pod(0, "n1", crashLoop(137, "OOMKilled", "", 1)))
			db.w.setReady(0)
			c.update(db.w.objects())
			c.update(trafficUnreadySlice(c, "postgres", db.pods...))
			for _, tier := range tiers {
				c.after(5 * time.Second)
				unreadyTier(c, tier)
			}
			c.after(8 * time.Minute)
		},
	}
}
