package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// advNetworkScenarios are adversarial stories about names, services,
// admission and registries: failures that look like a shared service
// but are not, and independent failures that look alike.
func advNetworkScenarios() []scenario {
	return []scenario{
		advDNSOneHostProbed(), advDNSOneHostUnprobed(),
		advDNSSharedHostMixed(), advInitWaitSelector(),
		advWebhookTimeoutManyNamespaces(), advSameErrorDifferentTargets(),
	}
}

const advNXDomain = "dial tcp: lookup sandbox.partner-gateway.example on " +
	"10.96.0.10:53: no such host"

// advDNSCast lists cluster DNS, checkout and an unrelated cart and
// returns checkout. Every cluster DNS probe the scenarios record
// succeeds: CoreDNS answers.
func advDNSCast(c *cluster, env []corev1.EnvVar) *workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	dns := c.deployment("kube-system", "coredns",
		"registry.k8s.io/coredns/coredns:v1.11.1", 2)
	checkout := c.deployment("shop", "checkout",
		"registry.example.com/checkout:3.0", 2)
	configTemplate(checkout, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = env
	})
	cart := c.deployment("shop", "cart", "registry.example.com/cart:3.1", 2)
	for _, w := range []*workload{dns, checkout, cart} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.probe(kube.ClusterDNS, "")
	return checkout
}

func advDNSRounds(
	c *cluster, w *workload, message string, probed inventory.EntityID,
	failure string,
) {
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.probe(kube.ClusterDNS, "")
		if probed.Name != "" {
			c.probe(probed, failure)
		}
		c.update(w.pod(0, "n1", crashLoop(1, "Error", message, restarts)),
			w.pod(1, "n2", crashLoop(1, "Error", message, restarts)))
		w.setReady(0)
		c.update(w.objects())
		c.after(time.Minute)
	}
}

// advDNSOneHostProbed: checkout is configured with a partner host that
// no longer exists. kwatch's probe of that host fails with "no such
// host", CoreDNS and its own probe are healthy, and cart resolves other
// names. The unresolvable host is the root; cluster DNS is not.
func advDNSOneHostProbed() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-dns-nxdomain-probed",
			Description: "One workload crashes on NXDOMAIN for the " +
				"external host in its configuration; the probe of that " +
				"host fails; CoreDNS and its probe are healthy.",
			Root:        "external-endpoint//sandbox.partner-gateway.example:443",
			Tier:        "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"cluster-dns//cluster-dns",
				"deployment/kube-system/coredns", "node//n1", "node//n2",
				"deployment/shop/checkout"},
		},
		build: func(c *cluster) {
			w := advDNSCast(c, []corev1.EnvVar{{Name: "PARTNER_URL",
				Value: "https://sandbox.partner-gateway.example:443/v2"}})
			c.after(time.Minute)
			host := inventory.CoreID(kube.KindExternalEndpoint, "",
				"sandbox.partner-gateway.example:443")
			advDNSRounds(c, w, advNXDomain, host,
				"dial tcp: lookup sandbox.partner-gateway.example on "+
					"10.96.0.10:53: no such host")
		},
	}
}

// advDNSOneHostUnprobed: the same crash, but nothing names the host
// outside the log line, so there is no probe. One workload failing on
// one name while cluster DNS answers is that workload's problem.
func advDNSOneHostUnprobed() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-dns-nxdomain-unprobed",
			Description: "One workload crashes on NXDOMAIN for an " +
				"external host that only its log names; CoreDNS and its " +
				"probe are healthy.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"cluster-dns//cluster-dns",
				"deployment/kube-system/coredns", "node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			w := advDNSCast(c, nil)
			c.after(time.Minute)
			advDNSRounds(c, w, advNXDomain, inventory.EntityID{}, "")
		},
	}
}

// advDNSSharedHostMixed: two workloads in two namespaces crash on the
// same vanished vendor host, one printing the Go resolver's text and
// one the Node resolver's. Cluster DNS is healthy. They are one vendor
// outage, but their error text differs; either two incidents or one
// grouped incident is honest, and neither may blame cluster DNS.
func advDNSSharedHostMixed() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-dns-nxdomain-two-namespaces",
			Description: "Two workloads in different namespaces fail on " +
				"the same vanished vendor host with different resolver " +
				"messages while cluster DNS is healthy.",
			Root:        "deployment/shop/checkout",
			OtherRoots:  []string{"deployment/billing/invoicer"},
			Tier:        "notify",
			MaxMessages: 4,
			MustNotBlame: []string{"cluster-dns//cluster-dns",
				"deployment/kube-system/coredns", "node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			checkout := advDNSCast(c, nil)
			invoicer := c.deployment("billing", "invoicer",
				"registry.example.com/invoicer:2.1", 2)
			c.list(invoicer.objects())
			c.list(invoicer.pod(0, "n1"), invoicer.pod(1, "n2"))
			c.after(time.Minute)
			node := "getaddrinfo ENOTFOUND api.vendor-x.example"
			goText := "dial tcp: lookup api.vendor-x.example on " +
				"10.96.0.10:53: no such host"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.probe(kube.ClusterDNS, "")
				c.update(checkout.pod(0, "n1", crashLoop(1, "Error",
					goText, restarts)), checkout.pod(1, "n2",
					crashLoop(1, "Error", goText, restarts)))
				c.update(invoicer.pod(0, "n1", crashLoop(1, "Error",
					node, restarts)), invoicer.pod(1, "n2",
					crashLoop(1, "Error", node, restarts)))
				checkout.setReady(0)
				invoicer.setReady(0)
				c.update(checkout.objects())
				c.update(invoicer.objects())
				c.after(time.Minute)
			}
		},
	}
}

// advInitWaitSelector: the api pod's init container waits for the queue
// Service. Someone edited the Service's selector to a label no pod
// carries, so it has no endpoints although the queue pods are healthy.
// The Service is the root; the healthy queue is not.
func advInitWaitSelector() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-init-wait-selector-edited",
			Description: "An init container waits for a Service whose " +
				"selector was edited to match nothing; its backing pods " +
				"are healthy.",
			Root: "service/shop/queue", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"deployment/shop/queue",
				"deployment/shop/api", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			queue := c.deployment("shop", "queue",
				"registry.example.com/queue:3", 1)
			c.list(queue.objects())
			qp := queue.pod(0, "n1")
			svc := clusterService(c, "shop", "queue", 5672)
			c.list(qp, svc, clusterSlice(c, "shop", "queue", qp))
			api := c.deployment("shop", "api", "registry.example.com/api:4", 1)
			configTemplate(api, func(spec *corev1.PodSpec) {
				spec.InitContainers = []corev1.Container{{
					Name: "wait-for-queue", Image: "registry.example.com/wait:1",
					Env: []corev1.EnvVar{{Name: "QUEUE_ADDR",
						Value: "queue:5672"}},
				}}
			})
			c.list(api.objects())
			c.after(time.Minute)
			edited := last(c, svc)
			edited.Spec.Selector = map[string]string{"app": "queue-v2"}
			c.update(edited)
			c.update(clusterSlice(c, "shop", "queue"))
			started := c.now
			for range 8 {
				c.update(stuckPod(api, started))
				c.after(time.Minute)
			}
		},
	}
}

func advDBSlice(
	c *cluster, namespace, name string, ready bool, pods ...*corev1.Pod,
) *discoveryv1.EndpointSlice {
	slice := clusterSlice(c, namespace, name, pods...)
	port := int32(5432)
	slice.Ports = []discoveryv1.EndpointPort{{Port: &port}}
	for i := range slice.Endpoints {
		slice.Endpoints[i].Conditions.Ready = boolPtr(ready)
	}
	return slice
}

func advDBService(
	c *cluster, namespace, name, ip string,
) *corev1.Service {
	svc := clusterService(c, namespace, name, 5432)
	svc.Spec.ClusterIP = ip
	return svc
}

// advSameErrorDifferentTargets: two clients crash with the same
// "connection refused" on port 5432, each dialing the address of its own
// database Service. shop's db-a crash-loops on a bad password setting;
// billing's db-b is healthy but its Service selector was edited to match
// nothing. Same words, two unrelated failures.
func advSameErrorDifferentTargets() scenario {
	return scenario{
		expect: expectation{
			Name: "adv-same-error-different-targets",
			Description: "Two clients refuse to connect on the same port " +
				"to two different database Services that fail for " +
				"unrelated reasons.",
			Root:        "deployment/shop/db-a",
			OtherRoots:  []string{"service/billing/db-b"},
			Tier:        "notify",
			MaxMessages: 4,
			MustNotBlame: []string{"deployment/shop/api-a",
				"deployment/billing/api-b", "node//n1", "node//n2"},
		},
		build: buildAdvSameError,
	}
}

func buildAdvSameError(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	dbA := c.deployment("shop", "db-a", "registry.example.com/pg:15", 1)
	dbB := c.deployment("billing", "db-b", "registry.example.com/pg:15", 1)
	apiA := c.deployment("shop", "api-a", "registry.example.com/api:2", 2)
	apiB := c.deployment("billing", "api-b", "registry.example.com/api:2", 2)
	for _, w := range []*workload{dbA, dbB, apiA, apiB} {
		c.list(w.objects())
	}
	podA, podB := dbA.pod(0, "n1"), dbB.pod(0, "n2")
	svcB := advDBService(c, "billing", "db-b", "10.96.18.40")
	c.list(podA, podB, advDBService(c, "shop", "db-a", "10.96.12.5"), svcB,
		advDBSlice(c, "shop", "db-a", true, podA),
		advDBSlice(c, "billing", "db-b", true, podB))
	for _, w := range []*workload{apiA, apiB} {
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(3 * time.Minute)
	edited := last(c, svcB)
	edited.Spec.Selector = map[string]string{"app": "db-b-v2"}
	c.update(edited)
	c.update(advDBSlice(c, "billing", "db-b", true))
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(dbA.pod(0, "n1", crashLoop(1, "Error", "FATAL: password "+
			"authentication failed for user \"postgres\"", restarts)))
		dbA.setReady(0)
		c.update(dbA.objects())
		c.update(advDBSlice(c, "shop", "db-a", false, dbA.pod(0, "n1")))
		for i, node := range []string{"n1", "n2"} {
			c.update(apiA.pod(i, node, crashLoop(1, "Error",
				"dial tcp 10.96.12.5:5432: connect: connection refused",
				restarts)))
			c.update(apiB.pod(i, node, crashLoop(1, "Error",
				"dial tcp 10.96.18.40:5432: connect: connection refused",
				restarts)))
		}
		apiA.setReady(0)
		apiB.setReady(0)
		c.update(apiA.objects())
		c.update(apiB.objects())
		c.after(time.Minute)
	}
}
