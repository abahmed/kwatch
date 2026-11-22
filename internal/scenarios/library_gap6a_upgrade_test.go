package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap6aScenarios are failures that a cluster upgrade or a version change
// causes: removed API versions, changed defaults, dropped providers.
func gap6aScenarios() []scenario {
	return []scenario{
		gap6aRemovedAPIVersion(), gap6aWebhookDefaultsFail(),
		gap6aLegacyCommonName(), gap6aExecProbeTimeout(),
		gap6aSpreadSkewUpgrade(), gap6aControllerManagerProbe(),
		gap6aSchedulerConfigPlugin(), gap6aCloudProviderRemoved(),
		gap6aOwnerRefNamespace(), gap6aRegistryRepoMoved(),
	}
}

// gap6Rollouts rolls out every workload of fleet at once and plays the
// ReplicaSets' FailedCreate warnings with message once a minute, rounds
// times.
func gap6Rollouts(
	c *cluster, fleet []*workload, message string, rounds int32,
) {
	for _, w := range fleet {
		rs := w.rollout(func(spec *corev1.PodSpec) {
			spec.Containers[0].Env = []corev1.EnvVar{{Name: "REV",
				Value: "2"}}
		})
		d := w.deployment
		d.Status.UpdatedReplicas = 0
		d.Status.Conditions = []appsv1.DeploymentCondition{{
			Type: appsv1.DeploymentReplicaFailure, Status: "True",
			Reason: "FailedCreate", Message: message,
			LastTransitionTime: metav1.NewTime(c.now)}}
		c.update(d)
		c.create(rs)
	}
	for m := int32(1); m <= rounds; m++ {
		for _, w := range fleet {
			g2Event(c, w.replicaSet, "ReplicaSet", "FailedCreate", message,
				"replicaset-controller", m)
		}
		c.after(time.Minute)
	}
}

// gap6aRemovedAPIVersion: the upgrade stopped serving the beta Ingress
// API; a controller built for it cannot list what it watches and exits.
func gap6aRemovedAPIVersion() scenario {
	return scenario{
		expect: g2Exp("upgrade-removes-api-version-controller-crashes",
			"After a cluster upgrade a controller built for a beta API "+
				"version that is no longer served exits at start because "+
				"it cannot list it.",
			"deployment/ingress-system/legacy-controller", "notify",
			"node//n1", "node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("ingress-system", "legacy-controller",
				"registry.example.com/legacy-controller:1.8", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(10 * time.Minute)
			message := "panic: failed to list *v1beta1.Ingress: the " +
				"server could not find the requested resource (get " +
				"ingresses.networking.k8s.io)"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(2, "Error", message,
					restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap6Webhook lists a fail-closed webhook with a ready backend and two
// workloads in other namespaces.
func gap6Webhook(c *cluster, name string) []*workload {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	guard := c.deployment("policy-system", "guard",
		"registry.example.com/guard:1", 1)
	c.list(guard.objects())
	c.list(guard.pod(0, "n1"))
	c.list(clusterService(c, "policy-system", "guard", 443),
		clusterSlice(c, "policy-system", "guard", guard.pod(0, "n1")))
	c.list(clusterValidatingHook(c, name, name+".policy.example.com",
		"policy-system", "guard"))
	return gap2Spread(c, []string{"shop/orders", "billing/ledger"},
		[]string{"n1", "n2"})
}

// gap6aWebhookDefaultsFail: a webhook configuration re-created under the
// v1 API now fails closed and times out after ten seconds; its backend is
// slow, which the old defaults ignored.
func gap6aWebhookDefaultsFail() scenario {
	return scenario{
		expect: g2Exp("webhook-v1-defaults-fail-closed-after-upgrade",
			"A webhook configuration migrated to the v1 API now fails "+
				"closed with a ten second timeout; its slow backend "+
				"makes pod creation fail in two namespaces.",
			"validatingwebhookconfiguration//legacy-labels", "page",
			"node//n1", "deployment/policy-system/guard",
			"deployment/shop/orders").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			fleet := gap6Webhook(c, "legacy-labels")
			c.after(10 * time.Minute)
			gap6Rollouts(c, fleet, "Internal error occurred: failed "+
				"calling webhook \"legacy-labels.policy.example.com\": "+
				"failed to call webhook: Post \"https://guard.policy-"+
				"system.svc:443/validate?timeout=10s\": context deadline "+
				"exceeded", 6)
		},
	}
}

// gap6aLegacyCommonName: the upgrade moved to a Go version that no longer
// accepts a certificate that names its host only in the Common Name.
func gap6aLegacyCommonName() scenario {
	return scenario{
		expect: g2Exp("webhook-certificate-common-name-only-after-upgrade",
			"After an upgrade the API server rejects a webhook's "+
				"serving certificate that has no subject alternative "+
				"name; pod creation fails in two namespaces.",
			"validatingwebhookconfiguration//cert-policy", "page",
			"node//n1", "deployment/policy-system/guard",
			"deployment/shop/orders").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			fleet := gap6Webhook(c, "cert-policy")
			c.after(10 * time.Minute)
			gap6Rollouts(c, fleet, "Internal error occurred: failed "+
				"calling webhook \"cert-policy.policy.example.com\": "+
				"failed to call webhook: Post \"https://guard.policy-"+
				"system.svc:443/validate?timeout=10s\": x509: "+
				"certificate relies on legacy Common Name field, use "+
				"SANs instead", 6)
		},
	}
}

// gap6aExecProbeTimeout: since the upgrade the kubelet enforces the
// one-second timeout of exec probes; a database check that takes three
// seconds now fails and the container is restarted.
func gap6aExecProbeTimeout() scenario {
	return scenario{
		expect: g2Exp("upgrade-enforces-exec-probe-timeout",
			"After an upgrade an exec liveness probe that takes three "+
				"seconds exceeds its one-second timeout; the kubelet "+
				"restarts the container again and again.",
			"deployment/shop/api", "notify", "node//n1", "node//n2").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].LivenessProbe = &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{Exec: &corev1.
						ExecAction{Command: []string{"/healthcheck.sh"}}},
					PeriodSeconds: 10}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				for i, node := range []string{"n1", "n2"} {
					pod := w.pod(i, node, crashLoop(137, "Error", "",
						restarts))
					c.update(pod)
					gapKubelet(c, pod, "Unhealthy", "Liveness probe "+
						"failed: command timed out: \"/healthcheck.sh\" "+
						"timed out after 1s", restarts)
				}
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap6aSpreadSkewUpgrade: since the upgrade the spread skew counts only
// the nodes the pod's nodeSelector matches; a scale-up of a Deployment
// with a zone constraint finds no placement.
func gap6aSpreadSkewUpgrade() scenario {
	return scenario{
		expect: g2Exp("upgrade-changes-topology-spread-counting",
			"After an upgrade the topology spread constraint counts "+
				"only nodes matching the selector; the scaled-up pods "+
				"are Pending with \"didn't match pod topology spread "+
				"constraints\". The pod spec's constraint explains it, "+
				"not the scheduling reason alone.",
			"deployment/shop/api", "notify", "node//n1", "node//n2",
			"node//n3").tail(10 * time.Minute),
		build: func(c *cluster) {
			var pool []runtime.Object
			for i, zone := range []string{"zone-a", "zone-b", "zone-c"} {
				node := c.node("n"+itoa(i+1), zone)
				node.Labels["pool"] = "web"
				pool = append(pool, node)
			}
			c.list(pool...)
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.NodeSelector = map[string]string{"pool": "web"}
				spec.TopologySpreadConstraints = []corev1.
					TopologySpreadConstraint{{MaxSkew: 1,
					TopologyKey:       corev1.LabelTopologyZone,
					WhenUnsatisfiable: corev1.DoNotSchedule}}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 4
			w.setReady(2)
			c.update(w.objects())
			message := "0/3 nodes are available: 3 node(s) didn't match " +
				"pod topology spread constraints (missing required " +
				"label). preemption: 0/3 nodes are available: 3 " +
				"Preemption is not helpful for scheduling."
			c.create(w.pod(2, "", pendingUnscheduled(message)),
				w.pod(3, "", pendingUnscheduled(message)))
			gap5aPending(c, w, []int{2, 3}, message, 8)
		},
	}
}

// gap6aControllerManagerProbe: a customised static pod manifest keeps an
// HTTP probe on the port the controller manager no longer serves; the
// kubelet restarts it repeatedly and controllers stall in between.
func gap6aControllerManagerProbe() scenario {
	return scenario{
		expect: g2Exp("controller-manager-static-pod-probe-port-removed",
			"A static pod manifest for the controller manager probes a "+
				"port the upgraded component no longer serves; the "+
				"kubelet restarts it again and again.",
			"controller-manager//kube-controller-manager", "page",
			"node//n1").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			cm := c.deployment("kube-system", "kube-controller-manager",
				"registry.k8s.io/kube-controller-manager:v1.30.2", 1)
			controlPlaneLabels(cm, "kube-controller-manager")
			c.list(cm.objects())
			c.list(cm.pod(0, "n1"))
			c.probe(kube.ControllerManager, "")
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				pod := cm.pod(0, "n1", crashLoop(137, "Error", "", restarts))
				c.update(pod)
				gapKubelet(c, pod, "Unhealthy", "Liveness probe failed: "+
					"Get \"http://127.0.0.1:10252/healthz\": dial tcp "+
					"127.0.0.1:10252: connect: connection refused",
					restarts)
				c.probe(kube.ControllerManager, "leader lease not renewed "+
					"for 1m20s")
				c.after(time.Minute)
				c.probe(kube.ControllerManager, "")
			}
		},
	}
}

// gap6aSchedulerConfigPlugin: the scheduler's configuration still names a
// plugin the upgrade removed; it exits at start and new pods stay Pending
// without a single event.
func gap6aSchedulerConfigPlugin() scenario {
	return scenario{
		expect: g2Exp("scheduler-config-names-removed-plugin",
			"A custom scheduler configuration names a plugin removed in "+
				"the upgrade; the scheduler crash-loops and new pods "+
				"stay Pending with no scheduling events.",
			"scheduler//kube-scheduler", "page", "node//n1", "node//n2",
			"deployment/shop/cart", "deployment/shop/search",
			"deployment/billing/invoices").budget(3).tail(5 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := controlPlaneFleet(c)
			sched := c.deployment("kube-system", "kube-scheduler",
				"registry.k8s.io/kube-scheduler:v1.32.0", 1)
			controlPlaneLabels(sched, "kube-scheduler")
			c.list(sched.objects())
			c.list(sched.pod(0, "n1"))
			c.probe(kube.Scheduler, "")
			c.after(time.Minute)
			for round := int32(1); round <= 4; round++ {
				c.update(sched.pod(0, "n1", crashLoop(1, "Error", "failed "+
					"to create scheduler: couldn't create scheduler: "+
					"plugin \"AzureDiskLimits\" does not exist", round)))
				sched.setReady(0)
				c.update(sched.objects())
				c.probe(kube.Scheduler, "leader lease not renewed for "+
					itoa(int(round)+1)+"m5s")
				if round == 2 {
					for _, w := range fleet {
						setReplicas(w, 3)
						w.setReady(2)
						c.update(w.objects())
						c.create(w.pod(2, "", notScheduled))
					}
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap6aCloudProviderRemoved: the upgrade removed the in-tree cloud
// provider and no external controller was installed; the nodes added
// since keep their uninitialized taint, so pods stay off them.
func gap6aCloudProviderRemoved() scenario {
	return scenario{
		expect: g2Exp("upgrade-without-cloud-controller-nodes-uninitialized",
			"After an upgrade removed the in-tree cloud provider with no "+
				"cloud controller installed, new nodes keep the "+
				"uninitialized taint and the pods that need them stay "+
				"Pending; the taint explains it, not the scheduling "+
				"reason alone.",
			"node//n3", "notify", "deployment/shop/web", "node//n1",
			"service/shop/web").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			taint := corev1.Taint{
				Key:   "node.cloudprovider.kubernetes.io/uninitialized",
				Value: "true", Effect: corev1.TaintEffectNoSchedule}
			for _, name := range []string{"n3", "n4"} {
				node := c.node(name, "zone-a")
				node.Spec.Taints = []corev1.Taint{taint}
				c.list(node)
			}
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			svc := clusterService(c, "shop", "web", 443)
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			c.list(svc)
			c.after(30 * time.Minute)
			*w.deployment.Spec.Replicas = 5
			w.setReady(2)
			c.update(w.objects())
			message := "0/4 nodes are available: 2 node(s) had untolerated " +
				"taint {node.cloudprovider.kubernetes.io/uninitialized: " +
				"true}, 2 Insufficient cpu. preemption: 0/4 nodes are " +
				"available: 2 Preemption is not helpful for scheduling, " +
				"2 No preemption victims found for incoming pod."
			for i := 2; i < 5; i++ {
				c.create(w.pod(i, "", pendingUnscheduled(message)))
			}
			gap5aPending(c, w, []int{2, 3, 4}, message, 8)
		},
	}
}
