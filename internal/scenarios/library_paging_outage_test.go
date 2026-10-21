package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// namespaceOutageWithServices: six of eight Deployments in shop, each
// behind a ClusterIP Service, crash-loop within seconds of each other
// with errors of their own. Each is its own cause (the Service does not
// tie it to the others), so it is still a namespace outage: one message.
func namespaceOutageWithServices() scenario {
	return scenario{
		expect: expectation{
			Name: "namespace-outage-with-services",
			Description: "Six of eight Deployments in one namespace, " +
				"each behind a Service, crash-loop within seconds of " +
				"each other with separate errors: one namespace " +
				"message, not one per workload.",
			Root:       "deployment/shop/app0",
			OtherRoots: outageRoots(6),
			Tier:       "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n0", "node//n1", "zone//zone-a"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			fleet, nodes := pagingFleet(c, 8, corev1.ServiceTypeClusterIP)
			crashServiced(c, fleet, nodes, 6, outageServicesWords)
		},
	}
}

// namespaceOutagePagesOnce: the same outage, but the Services are load
// balancers that users reach, so each failure is page tier. The
// namespace is paged once. The others are held at notify, so their
// reminders do not page the pagers again for an alert nobody opened.
func namespaceOutagePagesOnce() scenario {
	return scenario{
		expect: expectation{
			Name: "namespace-outage-pages-once",
			Description: "Six of eight user-facing Deployments in one " +
				"namespace crash-loop together: the namespace is paged " +
				"once, and the other five stay at notify.",
			Root:       "deployment/shop/app0",
			OtherRoots: outageRoots(6),
			Tier:       "page", MaxPages: 1, MaxMessages: 10,
			MustNotBlame: []string{"node//n0", "node//n1", "zone//zone-a"},
			Tail:         duration(7 * time.Hour),
		},
		build: func(c *cluster) {
			fleet, nodes := pagingFleet(c, 8,
				corev1.ServiceTypeLoadBalancer)
			crashServiced(c, fleet, nodes, 6, outagePagesOnceWords)
		},
	}
}

// twoTeamsTwoPages: two user-facing outages of two teams (namespaces)
// begin in the same moment. They are not one story: each team is paged.
func twoTeamsTwoPages() scenario {
	return scenario{
		expect: expectation{
			Name: "two-teams-two-pages",
			Description: "A user-facing outage in billing and another " +
				"in shop begin together: each team is paged, because " +
				"one page does not stand for another team's outage.",
			Root:       "deployment/billing/api",
			OtherRoots: []string{"deployment/shop/api"},
			Tier:       "page", MaxPages: 2, MaxMessages: 4,
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			type app struct {
				w          *workload
				node, ns   string
				dependency string
			}
			var apps []app
			for i, ns := range []string{"billing", "shop"} {
				node := "n" + string(rune('0'+i))
				c.list(c.node(node, "zone-a"))
				w := c.deployment(ns, "api",
					"registry.example.com/api:1.0", 1)
				c.list(w.deployment, w.replicaSet)
				p := w.pod(0, node)
				c.list(p)
				svc := clusterService(c, ns, "api", 8080)
				svc.Spec.Type = corev1.ServiceTypeLoadBalancer
				c.list(svc, clusterSlice(c, ns, "api", p))
				apps = append(apps, app{w, node, ns,
					outageDependencies[outageTwoTeamsWords+i]})
			}
			c.after(2 * time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for _, a := range apps {
					c.update(a.w.pod(0, a.node, crashLoop(1, "Error",
						"fatal: "+a.dependency+" lost its own dependency",
						restarts)))
					a.w.setReady(0)
					c.update(a.w.objects())
					c.update(clusterSlice(c, a.ns, "api"))
				}
				c.after(20 * time.Second)
			}
		},
	}
}
