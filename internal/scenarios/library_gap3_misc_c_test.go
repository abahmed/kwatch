package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap3Hairpin: after a CNI configuration change a single-replica app can
// no longer reach itself through its own Service address; it times out
// and exits.
func gap3Hairpin() scenario {
	return scenario{
		expect: g2Exp("pod-cannot-reach-own-service-address",
			"A single-replica app times out connecting to its own "+
				"Service address while its endpoints are ready and no "+
				"policy changed.",
			"unknown", "notify", "service/shop/ledger", "node//n1",
			"deployment/shop/ledger").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "ledger",
				"registry.example.com/ledger:4", 1)
			c.list(w.objects())
			pod := w.pod(0, "n1")
			c.list(pod)
			svc := clusterService(c, "shop", "ledger", 8080)
			svc.Spec.ClusterIP = "10.96.22.5"
			svc.Spec.ClusterIPs = []string{"10.96.22.5"}
			c.list(svc, clusterSlice(c, "shop", "ledger", pod))
			c.after(2 * time.Minute)
			message := "level=fatal msg=\"self check\" err=\"dial tcp " +
				"10.96.22.5:8080: i/o timeout\""
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap3MountBlip: a new pod's first mount attempt fails for five seconds
// ("not registered") and then succeeds; the pod is ready within ten.
func gap3MountBlip() scenario {
	return scenario{
		expect: g2Quiet("failed-mount-not-registered-clears-quiet",
			"A new pod gets one FailedMount \"object not registered\" "+
				"warning for its projected volume and is ready five "+
				"seconds later."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(5 * time.Minute)
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			pod := w.pod(1, "n1", startedNow,
				gapPending(waiting("ContainerCreating", "")))
			c.create(pod)
			gapKubelet(c, pod, "FailedMount", "MountVolume.SetUp failed "+
				"for volume \"kube-api-access-x7k2p\" : object \""+
				pod.Namespace+"\"/\"kube-root-ca.crt\" not registered", 1)
			c.after(5 * time.Second)
			c.update(w.pod(1, "n1", startedNow))
			w.setReady(3)
			c.update(w.objects())
			c.after(10 * time.Minute)
		},
	}
}

// gap3NameserverLimit: the node's resolv.conf lists five nameservers; the
// kubelet warns on every pod that only three are applied. The apps work.
func gap3NameserverLimit() scenario {
	return scenario{
		expect: g2Quiet("pod-dns-nameserver-limit-notice",
			"The kubelet warns for each of three healthy pods that the "+
				"node lists more than three nameservers and some were "+
				"omitted."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 3)
			c.list(w.objects())
			for i := range 3 {
				pod := w.pod(i, "n1")
				c.list(pod)
				gapKubelet(c, pod, "DNSConfigForming", "Nameserver "+
					"limits were exceeded, some nameservers have been "+
					"omitted, the applied nameserver line is: 10.96.0.10 "+
					"10.0.0.2 8.8.8.8", 1)
			}
			c.after(10 * time.Minute)
		},
	}
}

// gap3DNSLatency: three apps' pods fail a readiness probe now and then
// for an hour; nothing else is wrong that the cluster can show. The
// cause is outside what Kubernetes reports.
func gap3DNSLatency() scenario {
	return scenario{
		expect: g2Exp("sporadic-probe-timeouts-no-event",
			"Pods of three workloads fail a readiness probe by timeout "+
				"now and then for an hour; CoreDNS and the nodes look "+
				"healthy and nothing else is reported.",
			"unknown", "digest", "cluster-dns//cluster-dns",
			"node//n1", "node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, []string{"n1", "n2"})
			c.probe(kube.ClusterDNS, "")
			for round := int32(1); round <= 12; round++ {
				c.after(5 * time.Minute)
				w := fleet[int(round)%3]
				pod := w.pod(int(round)%2, "n"+itoa(int(round)%2+1), notReady)
				c.update(pod)
				gapKubelet(c, pod, "Unhealthy", "Readiness probe failed: "+
					"Get \"http://10.244.1.9:8080/ready\": context "+
					"deadline exceeded (Client.Timeout exceeded while "+
					"awaiting headers)", 1)
				c.after(5 * time.Second)
				c.update(w.pod(int(round)%2, "n"+itoa(int(round)%2+1)))
				c.probe(kube.ClusterDNS, "")
			}
		},
	}
}
