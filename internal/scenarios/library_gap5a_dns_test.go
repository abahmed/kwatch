package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap5aDNSScenarios are servers that cannot be reached by a bind address
// or a Service protocol, and the DNS service of the cluster.
func gap5aDNSScenarios() []scenario {
	return []scenario{
		gap5aLoopbackBind(), gap5aIPv6OnlyBind(), gap5aServiceProtocol(),
		gap5aKubeDNSRecreated(), gap5aKubeDNSSelector(),
		gap5aUpstreamResolver(), gap5aUpgradeOverwritesCorefile(),
	}
}

// gap5aProbe is a readiness probe of the api container on port 8080.
func gap5aProbe(spec *corev1.PodSpec) {
	spec.Containers[0].ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/ready", Port: intstr.FromInt32(8080)}},
		PeriodSeconds: 10,
	}
}

// gap5aBindRelease is a release that makes api listen on listen; its
// readiness probe, which dials the pod address, is refused.
func gap5aBindRelease(e expectation, listen string, caller bool) scenario {
	return g2Release(e.tail(5*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			gap5aProbe(spec)
			spec.Containers[0].Env = []corev1.EnvVar{{Name: "LISTEN",
				Value: listen}}
		},
		func(c *cluster, w *workload) {
			var web *workload
			if caller {
				web = c.deployment("shop", "web",
					"registry.example.com/web:3", 1)
				c.list(web.objects())
				c.list(web.pod(0, "n1"))
				// The caller dials the Service of api by its address.
				svc := clusterService(c, "shop", "api", 8080)
				svc.Spec.ClusterIP = "10.96.22.5"
				c.list(svc, clusterSlice(c, "shop", "api"))
			}
			gapRounds(c, w, 8, 30*time.Second, func(int32) podState {
				return notReady
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "Unhealthy", "Readiness probe failed: "+
					"Get \"http://10.244.1.9:8080/ready\": dial tcp "+
					"10.244.1.9:8080: connect: connection refused", round)
				if web != nil {
					c.update(web.pod(0, "n1", crashLoop(1, "Error",
						"dial tcp 10.96.22.5:8080: connect: connection "+
							"refused", round)))
					web.setReady(0)
					c.update(web.objects())
				}
			})
		})
}

// gap5aLoopbackBind: a release binds the server to 127.0.0.1; the pod
// runs but is refused on its own address, so it never becomes ready and a
// caller's connections to the Service are refused.
func gap5aLoopbackBind() scenario {
	return gap5aBindRelease(g2Exp("release-server-binds-loopback",
		"A release makes the server listen on 127.0.0.1 only; the new "+
			"pod runs but its readiness probe is refused on the pod "+
			"address and a caller's connections are refused.",
		"deployment/shop/api", "notify", "deployment/shop/web",
		"service/shop/api", "node//n1", "node//n2"), "127.0.0.1:8080", true)
}

// gap5aIPv6OnlyBind: a release makes the server listen on an IPv6 address
// only, while the cluster's pod addresses are IPv4.
func gap5aIPv6OnlyBind() scenario {
	return gap5aBindRelease(g2Exp("release-server-binds-other-ip-family",
		"A release makes the server listen on an IPv6 address only "+
			"while pod addresses are IPv4; the readiness probe is "+
			"refused from the first start.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		"[::1]:8080", false)
}

// gap5aServiceProtocol: a Service declares UDP for an application that
// speaks TCP; its endpoints are ready and its callers time out.
func gap5aServiceProtocol() scenario {
	return scenario{
		expect: g2Exp("service-protocol-udp-for-tcp-app",
			"A Service declares protocol UDP for a TCP application; its "+
				"endpoints are ready and a caller times out connecting "+
				"to it.",
			"service/shop/api", "notify", "deployment/shop/api",
			"deployment/shop/web", "node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			api := c.deployment("shop", "api", "registry.example.com/api:4", 2)
			c.list(api.objects())
			pods := []*corev1.Pod{api.pod(0, "n1"), api.pod(1, "n2")}
			c.list(pods[0], pods[1])
			svc := clusterService(c, "shop", "api", 8080)
			svc.Spec.Ports[0].Protocol = corev1.ProtocolUDP
			svc.Spec.ClusterIP = "10.96.22.5"
			c.list(svc, clusterSlice(c, "shop", "api", pods...))
			web := c.deployment("shop", "web", "registry.example.com/web:3", 2)
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n2"))
			c.after(2 * time.Minute)
			message := "Get \"http://api.shop.svc:8080/items\": dial tcp " +
				"10.96.22.5:8080: i/o timeout"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(web.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), web.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				web.setReady(0)
				c.update(web.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap5aDNS lists CoreDNS (two ready pods), kube-dns and two callers.
func gap5aDNS(c *cluster) (*workload, []*workload) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
	dns := c.deployment("kube-system", "coredns",
		"registry.k8s.io/coredns/coredns:v1.11.1", 2)
	c.list(dns.objects())
	c.list(dns.pod(0, "n1"), dns.pod(1, "n2"))
	svc := clusterService(c, "kube-system", "kube-dns", 53)
	svc.Spec.ClusterIP = "10.96.0.10"
	c.list(svc, clusterSlice(c, "kube-system", "kube-dns", dns.pod(0, "n1"),
		dns.pod(1, "n2")))
	fleet := gap2Spread(c, []string{"shop/cart", "billing/invoices"},
		[]string{"n1", "n2"})
	c.probe(kube.ClusterDNS, "")
	return dns, fleet
}

// gap5aLookups crashes both replicas of the callers on message, once a
// minute for rounds minutes.
func gap5aLookups(c *cluster, fleet []*workload, message string, rounds int32) {
	for round := int32(1); round <= rounds; round++ {
		gap2CrashPods(c, fleet, []string{"n1", "n2"}, []int{0, 1}, message,
			round)
		c.after(time.Minute)
	}
}

// gap5aKubeDNSRecreated: the kube-dns Service is deleted and created again
// with a new ClusterIP; pods that started before keep the old address in
// their resolver and time out, pods that start now work.
func gap5aKubeDNSRecreated() scenario {
	return scenario{
		expect: g2Exp("kube-dns-service-recreated-new-cluster-ip",
			"The kube-dns Service is recreated with a new ClusterIP; "+
				"older pods time out on lookups at the old address, "+
				"newer pods resolve.",
			"service/kube-system/kube-dns", "notify", "node//n1",
			"node//n2", "deployment/kube-system/coredns",
			"deployment/shop/cart", "deployment/billing/invoices").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			_, fleet := gap5aDNS(c)
			c.after(10 * time.Minute)
			old := clusterService(c, "kube-system", "kube-dns", 53)
			c.remove(old)
			fresh := clusterService(c, "kube-system", "kube-dns", 53)
			fresh.Spec.ClusterIP = "10.96.0.11"
			fresh.CreationTimestamp = metav1.NewTime(c.now)
			c.create(fresh)
			gap5aLookups(c, fleet, "dial tcp: lookup db.shop.svc.cluster."+
				"local on 10.96.0.10:53: read udp 10.244.1.5:48762->"+
				"10.96.0.10:53: i/o timeout", 6)
		},
	}
}

// gap5aKubeDNSSelector: the kube-dns Service's selector is edited to a
// label no CoreDNS pod has; it has no endpoints while CoreDNS runs.
func gap5aKubeDNSSelector() scenario {
	return scenario{
		expect: g2Exp("kube-dns-service-selector-matches-nothing",
			"The kube-dns Service's selector is edited to a label that "+
				"no CoreDNS pod has; it has no endpoints, lookups fail "+
				"everywhere and CoreDNS pods stay Ready.",
			"service/kube-system/kube-dns", "page", "node//n1",
			"node//n2", "deployment/kube-system/coredns",
			"deployment/shop/cart", "deployment/billing/invoices").
			budget(3),
		build: func(c *cluster) {
			_, fleet := gap5aDNS(c)
			c.after(10 * time.Minute)
			svc := clusterService(c, "kube-system", "kube-dns", 53)
			svc.Spec.ClusterIP = "10.96.0.10"
			svc.Spec.Selector = map[string]string{"k8s-app": "kube-dns-v2"}
			c.update(svc, clusterSlice(c, "kube-system", "kube-dns"))
			for round := int32(1); round <= 6; round++ {
				c.probe(kube.ClusterDNS, "lookup kubernetes.default.svc."+
					"cluster.local on 10.96.0.10:53: read udp 10.244.1."+
					"7:41234->10.96.0.10:53: i/o timeout")
				gap2CrashPods(c, fleet, []string{"n1", "n2"}, []int{0, 1},
					"dial tcp: lookup db.shop.svc.cluster.local on "+
						"10.96.0.10:53: i/o timeout", round)
				c.after(30 * time.Second)
			}
		},
	}
}

// gap5aUpstreamResolver: the resolver CoreDNS forwards to stops answering;
// cluster names resolve, every external name fails. The apps that call
// partners crash; the DNS probe of a cluster name stays healthy.
func gap5aUpstreamResolver() scenario {
	return scenario{
		expect: g2Exp("coredns-upstream-resolver-unreachable",
			"CoreDNS's upstream resolver stops answering; cluster names "+
				"resolve but every external name fails with \"server "+
				"misbehaving\" in several namespaces.",
			"deployment/kube-system/coredns", "notify", "node//n1",
			"node//n2", "deployment/shop/cart",
			"deployment/billing/invoices").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			_, fleet := gap5aDNS(c)
			c.after(10 * time.Minute)
			gap5aLookups(c, fleet, "Get \"https://api.partner.example/v1\": "+
				"dial tcp: lookup api.partner.example on 10.96.0.10:53: "+
				"server misbehaving", 6)
		},
	}
}

// gap5aUpgradeOverwritesCorefile: a cluster upgrade reconciles the CoreDNS
// add-on and replaces the ConfigMap an operator customised with a stub
// domain; CoreDNS restarts healthy but the internal domain no longer
// resolves.
func gap5aUpgradeOverwritesCorefile() scenario {
	return scenario{
		expect: g2Exp("upgrade-overwrites-coredns-stub-domain",
			"A cluster upgrade replaces the customised CoreDNS "+
				"ConfigMap and its stub domain disappears; CoreDNS "+
				"restarts healthy and the internal domain stops "+
				"resolving.",
			"configmap/kube-system/coredns", "notify", "node//n1",
			"node//n2", "deployment/shop/cart",
			"deployment/billing/invoices").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
			cm := configMap(c, "kube-system", "coredns", map[string]string{
				"Corefile": ".:53 {\n    errors\n    health\n    " +
					"kubernetes cluster.local\n    forward . " +
					"/etc/resolv.conf\n    cache 30\n}\n" +
					"corp.internal:53 {\n    forward . 10.1.0.2\n}\n"})
			dns := c.deployment("kube-system", "coredns",
				"registry.k8s.io/coredns/coredns:v1.11.1", 2)
			configTemplate(dns, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{configVolume("config-volume",
					c.n("coredns"))}
			})
			c.list(cm, dns.deployment, dns.replicaSet)
			c.list(dns.pod(0, "n1"), dns.pod(1, "n2"))
			fleet := gap2Spread(c, []string{"shop/cart", "billing/invoices"},
				[]string{"n1", "n2"})
			c.after(10 * time.Minute)
			upgraded := last(c, cm)
			upgraded.Data["Corefile"] = ".:53 {\n    errors\n    health\n" +
				"    kubernetes cluster.local\n    forward . " +
				"/etc/resolv.conf\n    cache 30\n}\n"
			c.update(upgraded)
			c.update(dns.pod(0, "n1", startedNow), dns.pod(1, "n2",
				startedNow))
			c.after(time.Minute)
			gap5aLookups(c, fleet, "dial tcp: lookup billing-db.corp."+
				"internal on 10.96.0.10:53: no such host", 6)
		},
	}
}
