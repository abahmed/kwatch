package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// advIndependenceScenarios are adversarial stories about telling real
// from coincidental: independent faults on one node, look-alike names,
// silence that must stay silent, and evidence that is not there.
func advIndependenceScenarios() []scenario {
	return []scenario{
		advNodeLostAndAppCrash(), advLookalikeRedis(),
		advSilentExit(), advExit137NotOOM(),
		advQuotaFullQuiet(), advSecretRotationQuiet(),
		advSameNodeIndependent(), advClusterRoleBindingDeleted(),
		advTwoRegistriesIndependent(), advQuotaFullQuietLong(),
		advSecretInvisible(),
		advThrottledHealthyQuiet(), advRolloutInFlightNodeLost(),
		advNodeBlipThenBadRollout(), advTwoCausesLong(),
	}
}

// advNodeLostAndAppCrash: api's new image crash-loops on n1 and n3; three
// minutes later n2 stops reporting, which also takes down api's old pod
// there and replicas of two other workloads. The node loss and the bad
// release are unrelated; each is its own incident.
func advNodeLostAndAppCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-node-lost-and-app-crash",
			Description: "A bad release crash-loops while, unrelated, a " +
				"node that holds one of its old pods stops reporting.",
			Root: "deployment/shop/api", OtherRoots: []string{"node//n2"},
			Tier: "notify", MaxMessages: 6, MaxPages: 1,
			MustNotBlame: []string{"node//n1", "node//n3", "zone//zone-a",
				"deployment/shop/cart", "deployment/shop/search"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			n2 := c.node("n2", "zone-a")
			c.list(c.node("n1", "zone-a"), n2, c.node("n3", "zone-a"))
			nodes := []string{"n1", "n2", "n3"}
			fleet := nodeFleet(c, "shop",
				[]string{"api", "cart", "search"}, nodes)
			api := fleet[0]
			c.after(2 * time.Minute)
			old := []*corev1.Pod{api.pod(0, "n1"), api.pod(1, "n2"),
				api.pod(2, "n3")}
			rs := api.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/api:2.0"
			})
			c.update(api.deployment)
			c.create(rs)
			crash := "panic: assignment to entry in nil map"
			for restarts := int32(1); restarts <= 3; restarts++ {
				c.update(api.pod(0, "n1", startedNow, crashLoop(2, "Error",
					crash, restarts)), api.pod(1, "n3", startedNow,
					crashLoop(2, "Error", crash, restarts)))
				c.after(time.Minute)
			}
			nodeLose(c, n2)
			c.after(40 * time.Second)
			notReady(c, old[1])
			c.update(old[1])
			for _, w := range fleet[1:] {
				c.update(w.pod(1, "n2", notReady))
				w.setReady(2)
				c.update(w.objects())
			}
			for restarts := int32(4); restarts <= 8; restarts++ {
				c.update(api.pod(0, "n1", startedNow, crashLoop(2, "Error",
					crash, restarts)), api.pod(1, "n3", startedNow,
					crashLoop(2, "Error", crash, restarts)))
				c.after(time.Minute)
			}
		},
	}
}

// advLookalikeRedis: cart crashes on a refused connection to a managed
// cache outside the cluster whose host name starts with "redis". A
// retired in-cluster Service and Deployment also called redis sit
// scaled to zero. The word is shared; nothing else is.
func advLookalikeRedis() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-lookalike-redis",
			Description: "A workload crashes on a refused connection to " +
				"an external cache named redis-prod while an unused " +
				"in-cluster redis is scaled to zero.",
			Root: "deployment/shop/cart", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/redis",
				"service/shop/redis", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			legacy := c.deployment("shop", "redis",
				"registry.example.com/redis:6", 0)
			c.list(legacy.objects())
			c.list(clusterService(c, "shop", "redis", 6379),
				clusterSlice(c, "shop", "redis"))
			cart := c.deployment("shop", "cart",
				"registry.example.com/cart:4", 2)
			c.list(cart.objects())
			c.list(cart.pod(0, "n1"), cart.pod(1, "n2"))
			c.after(3 * time.Minute)
			message := "redis: dial tcp redis-prod.abc123.cache." +
				"example.com:6379: connect: connection refused"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(cart.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), cart.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				cart.setReady(0)
				c.update(cart.objects())
				c.after(time.Minute)
			}
		},
	}
}

// advSilentExit: both replicas exit 1 over and over with no message, no
// log line and nothing that changed. No cause exists to state.
func advSilentExit() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-silent-exit-no-evidence",
			Description: "A workload exits 1 repeatedly with no message, " +
				"no log and no change anywhere.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"zone//zone-a"},
		},
		build: func(c *cluster) {
			api, _ := accessFleet(c)
			c.after(5 * time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(api.pod(0, "n1", crashLoop(1, "Error", "",
					restarts)), api.pod(1, "n2", crashLoop(1, "Error", "",
					restarts)))
				api.setReady(0)
				c.update(api.objects())
				c.after(time.Minute)
			}
		},
	}
}

// advExit137NotOOM: containers are killed with exit code 137 whose
// reason is plain Error, not OOMKilled, with a memory limit far above
// their use. A SIGKILL says nothing about memory.
func advExit137NotOOM() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-exit137-not-oom",
			Description: "Containers exit with code 137 and reason Error " +
				"while their memory use is a fraction of the limit.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			api := c.deployment("shop", "api", "registry.example.com/api:4", 2)
			configTemplate(api, func(spec *corev1.PodSpec) {
				spec.Containers[0].Resources.Limits = corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				}
			})
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(3 * time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(api.pod(0, "n1", crashLoop(137, "Error", "",
					restarts)), api.pod(1, "n2", crashLoop(137, "Error",
					"", restarts)))
				api.setReady(0)
				c.update(api.objects())
				mib := float64(1 << 20)
				for i, node := range []string{"n1", "n2"} {
					pod := api.pod(i, node)
					c.emit(usage(c.now, kube.ContainerID(pod.Namespace,
						pod.Name, "app"), 210*mib))
				}
				c.after(time.Minute)
			}
		},
	}
}
