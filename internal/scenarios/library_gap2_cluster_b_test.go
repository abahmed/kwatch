package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap2ClusterBScenarios are cascades: one controller, priority class,
// certificate or API group whose fault reaches other namespaces.
func gap2ClusterBScenarios() []scenario {
	return []scenario{
		gap2KubeProxySyncFails(), gap2RunawayCronJob(),
		gap2PriorityChain(), gap2ControlPlaneCert(),
		gap2NamespaceAPIService(),
	}
}

// gap2KubeProxySyncFails: kube-proxy on n3 is running but its readiness
// check reports a stale sync; Services do not work from n3, so pods there
// time out on cluster addresses.
func gap2KubeProxySyncFails() scenario {
	return scenario{
		expect: g2Exp("kube-proxy-not-syncing-one-node",
			"kube-proxy runs on every node but its pod on one node is "+
				"not ready because it cannot program rules; pods there "+
				"time out on Service addresses.",
			"daemonset/kube-system/kube-proxy", "notify", "node//n3",
			"deployment/shop/web", "deployment/billing/invoices",
			"deployment/media/transcode", "cluster-dns//cluster-dns").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			ds := g2Daemon(c, "kube-system", "kube-proxy", nodes...)
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, nodes)
			c.after(2 * time.Minute)
			message := "Readiness probe failed: HTTP probe failed with " +
				"statuscode: 503"
			timeout := "dial tcp 10.96.14.7:5432: i/o timeout"
			for round := int32(1); round <= 6; round++ {
				setAgentCounts(ds, 3, 2)
				pod := agentPod(c, ds, "n3", notReady)
				c.update(ds, pod)
				gapKubelet(c, pod, "Unhealthy", message, round)
				if round >= 2 {
					gap2CrashPods(c, fleet, nodes, []int{2}, timeout,
						round-1)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap2RunawayCronJob: a CronJob run asks for 500 parallel pods; every
// node is full and the replicas other teams scale up stay Pending with
// "Too many pods".
func gap2RunawayCronJob() scenario {
	return scenario{
		expect: g2Exp("runaway-cronjob-fills-nodes",
			"A CronJob's run creates pods until every node is at its pod "+
				"limit; Deployments of three other namespaces cannot "+
				"place new replicas (\"Too many pods\").",
			"cronjob/batch/fanout", "notify", "deployment/shop/web",
			"deployment/billing/invoices", "deployment/media/transcode").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			names := []string{"n1", "n2", "n3"}
			for _, name := range names {
				node := c.node(name, "zone-a")
				node.Status.Allocatable[corev1.ResourcePods] =
					resource.MustParse("12")
				c.list(node)
			}
			victims := gap2Spread(c, []string{"shop/web",
				"billing/invoices", "media/transcode"}, names[:1])
			cron := scheduleCronJob(c, "batch", "fanout", "*/30 * * * *")
			c.list(cron)
			c.after(10 * time.Minute)
			job := activeJob(c, cron, "fanout-2925"+"1930", c.now)
			parallel := int32(500)
			job.Spec.Parallelism = &parallel
			c.create(job)
			for i := range 24 {
				pod := retryPod(c, job, i, "")
				pod.Spec.NodeName = c.n(names[i%3])
				c.create(pod)
			}
			c.after(time.Minute)
			message := "0/3 nodes are available: 3 Too many pods. " +
				"preemption: 0/3 nodes are available: 3 No preemption " +
				"victims found for incoming pod."
			for _, w := range victims {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
				c.create(w.pod(1, "", pendingUnscheduled(message)))
			}
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				for _, w := range victims {
					g2Event(c, last(c, w.pod(1, "")), "Pod",
						"FailedScheduling", message, "default-scheduler", m)
				}
			}
		},
	}
}

// gap2PriorityChain: a trainer of priority 1000 scales up and preempts
// workers of two namespaces; one worker's replacement preempts a render
// pod of a third, and the pods left without a node stay Pending.
func gap2PriorityChain() scenario {
	return scenario{
		expect: g2Exp("preemption-chain-across-namespaces",
			"A high-priority trainer's scale-up preempts pods of two "+
				"namespaces; their replacements preempt pods of a "+
				"third, and the evicted pods stay Pending.",
			"deployment/ml/trainer", "notify", "node//n1",
			"node//n2").budget(4).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			trainer := c.deployment("ml", "trainer",
				"registry.example.com/trainer:2", 1)
			worker := c.deployment("shop", "worker",
				"registry.example.com/worker:1", 2)
			ledger := c.deployment("billing", "ledger-batch",
				"registry.example.com/ledger:1", 2)
			render := c.deployment("media", "render",
				"registry.example.com/render:1", 2)
			for _, w := range []*workload{trainer, worker, ledger, render} {
				c.list(w.objects())
			}
			c.list(worker.pod(0, "n1", withPriority(500)),
				worker.pod(1, "n1", withPriority(500)),
				ledger.pod(0, "n2", withPriority(500)),
				ledger.pod(1, "n2", withPriority(500)),
				render.pod(0, "n2", withPriority(0)),
				render.pod(1, "n2", withPriority(0)),
				trainer.pod(0, "n1", withPriority(1000)))
			c.after(5 * time.Minute)
			boss := trainer.pod(0, "n1", withPriority(1000))
			name := boss.Namespace + "/" + boss.Name
			c.update(worker.pod(0, "n1", withPriority(500),
				preemptedFor(name)), worker.pod(1, "n1",
				withPriority(500), preemptedFor(name)),
				ledger.pod(0, "n2", withPriority(500), preemptedFor(name)))
			worker.setReady(0)
			ledger.setReady(1)
			c.update(worker.objects())
			c.update(ledger.objects())
			c.after(30 * time.Second)
			stuck := "0/2 nodes are available: 2 Insufficient cpu."
			c.create(worker.pod(2, "", withPriority(500),
				pendingUnscheduled(stuck)),
				ledger.pod(2, "", withPriority(500),
					pendingUnscheduled(stuck)))
			c.after(30 * time.Second)
			replacement := worker.pod(2, "n2", withPriority(500))
			c.update(replacement)
			c.update(render.pod(0, "n2", withPriority(0), preemptedFor(
				replacement.Namespace+"/"+replacement.Name)))
			render.setReady(1)
			c.update(render.objects())
			c.create(render.pod(2, "", withPriority(0),
				pendingUnscheduled(stuck)))
			c.after(3 * time.Minute)
		},
	}
}

// gap2ControlPlaneCert: the API server's serving certificate has
// expired; its health probe fails with an x509 error and the components
// that call it fail the same way.
func gap2ControlPlaneCert() scenario {
	return scenario{
		expect: g2Exp("control-plane-certificate-expired",
			"The API server's serving certificate has expired; kwatch's "+
				"probe of it fails with an x509 expiry error.",
			"apiserver//kube-apiserver", "page", "node//n1", "node//n2",
			"etcd//etcd"),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			controlPlaneFleet(c)
			c.probe(kube.APIServer, "")
			c.probe(kube.Etcd, "")
			c.after(time.Minute)
			for range 5 {
				c.probe(kube.APIServer, "Get \"https://10.96.0.1:443/"+
					"readyz\": tls: failed to verify certificate: "+
					"x509: certificate has expired or is not yet "+
					"valid: current time 2026-11-19T10:04:00Z is after "+
					"2026-11-19T09:12:41Z")
				c.probe(kube.Etcd, "")
				c.after(time.Minute)
			}
		},
	}
}

// gap2NamespaceAPIService: a namespace has been Terminating for two
// hours because the namespace controller cannot list every API group;
// the stale metrics APIService has no endpoints.
func gap2NamespaceAPIService() scenario {
	return scenario{
		expect: g2Exp("namespace-stuck-on-unavailable-apiservice",
			"A namespace stays Terminating because discovery fails for "+
				"the metrics API group, whose APIService has no "+
				"endpoints.",
			"apiservice//v1beta1.metrics.k8s.io", "notify",
			"namespace//old-shop").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(clusterService(c, "kube-system", "metrics-server", 10250),
				clusterSlice(c, "kube-system", "metrics-server"))
			c.list(clusterAPIService(c, "False", "MissingEndpoints",
				"endpoints for service/metrics-server in \"kube-system\" "+
					"have no addresses with port name \"https\""))
			ns := &corev1.Namespace{ObjectMeta: c.meta("", "old-shop")}
			deleting(&ns.ObjectMeta, c.now.Add(-2*time.Hour))
			ns.Spec.Finalizers = []corev1.FinalizerName{"kubernetes"}
			ns.Status.Phase = corev1.NamespaceTerminating
			ns.Status.Conditions = []corev1.NamespaceCondition{{
				Type:   corev1.NamespaceDeletionDiscoveryFailure,
				Status: corev1.ConditionTrue, Reason: "DiscoveryFailed",
				Message: "Discovery failed for some groups, 1 failing: " +
					"unable to retrieve the complete list of server " +
					"APIs: metrics.k8s.io/v1beta1: stale GroupVersion " +
					"discovery: metrics.k8s.io/v1beta1",
				LastTransitionTime: metav1.NewTime(
					c.now.Add(-2 * time.Hour)),
			}}
			c.list(ns)
			c.after(20 * time.Minute)
			c.update(ns.DeepCopy())
		},
	}
}
