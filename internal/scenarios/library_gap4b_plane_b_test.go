package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap4bPlaneBScenarios are stale control-plane state, slow drains, secret
// backlogs, restarts without gating, skewed load and runaway objects.
func gap4bPlaneBScenarios() []scenario {
	return []scenario{
		gap4bStaleEndpoints(), gap4bSlowDrainQuiet(), gap4bSecretBacklog(),
		gap4bStatefulSetRecreate(), gap4bSkewedLoad(), gap4bCronFlood(),
		gap4bEtcdIntermittent(),
	}
}

// gap4bStaleEndpoints: after a control-plane reconfiguration a Service's
// EndpointSlice still lists the addresses of two pods that no longer
// exist; callers dial them and fail while the real pods wait.
func gap4bStaleEndpoints() scenario {
	e := g2Exp("endpoint-slice-lists-pods-that-are-gone",
		"After a control-plane change a Service's EndpointSlice lists "+
			"pods that no longer exist and omits the running ones; "+
			"callers get \"no route to host\".", "unknown", "notify",
		"deployment/shop/payments", "node//n1", "node//n2",
		"external-endpoint//10.244.0.10:8080").budget(3).
		tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{expect: e, build: func(c *cluster) {
		c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
		payments := c.deployment("shop", "payments",
			"registry.example.com/payments:3", 2)
		c.list(payments.objects())
		c.list(payments.pod(0, "n1"), payments.pod(1, "n2"))
		c.list(clusterService(c, "shop", "payments", 8080))
		c.probe(kube.Etcd, "")
		gone := payments.pod(5, "n1")
		goneToo := payments.pod(6, "n2")
		c.list(clusterSlice(c, "shop", "payments", gone, goneToo))
		callers := gap2Spread(c, []string{"shop/cart", "billing/invoices"},
			[]string{"n1", "n2"})
		c.after(10 * time.Minute)
		c.probe(kube.Etcd, "[-]etcd failed: etcdserver: leader changed")
		message := "dial tcp 10.244.0.10:8080: connect: no route to host"
		for restarts := int32(1); restarts <= 5; restarts++ {
			gap2CrashPods(c, callers, []string{"n1", "n2"}, []int{0, 1},
				message, restarts)
			c.after(time.Minute)
		}
	}}
}

// gap4bSlowDrainQuiet: a node pool upgrade cordons n1; its pods have a
// ten-minute grace period and are Terminating for eight minutes, within
// it, while the Deployment holds pods of two ReplicaSets.
func gap4bSlowDrainQuiet() scenario {
	return scenario{
		expect: g2Quiet("drain-long-grace-period-within-grace-quiet",
			"A node is drained during an upgrade; its pods have a "+
				"ten-minute grace period and stay Terminating for "+
				"eight, while two ReplicaSets hold pods for half an "+
				"hour."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "worker",
				"registry.example.com/worker:4", 2)
			grace := int64(600)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.TerminationGracePeriodSeconds = &grace
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(5 * time.Minute)
			advCordon(c, c.node("n1", "zone-a"))
			c.update(w.pod(0, "n1", func(c *cluster, pod *corev1.Pod) {
				deleting(&pod.ObjectMeta, c.now)
			}), w.pod(1, "n1", func(c *cluster, pod *corev1.Pod) {
				deleting(&pod.ObjectMeta, c.now)
			}))
			c.after(8 * time.Minute)
			c.remove(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(25 * time.Minute)
		},
	}
}

// gap4bSecretBacklog: the controller that creates a credentials Secret
// for each new pod is scaled down while three namespaces roll out; their
// new pods wait for secrets that nobody creates.
func gap4bSecretBacklog() scenario {
	return scenario{
		expect: g2Exp("secret-controller-scaled-down-new-pods-wait",
			"The controller that creates per-pod secrets is scaled to "+
				"zero; new pods of three namespaces wait in "+
				"ContainerCreating on \"secret not found\" while old "+
				"pods run. Nothing in the cluster ties a secret name to "+
				"the controller that creates it.",
			"unknown", "notify", "node//n1",
			"deployment/credentials/secret-sync").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			sync := c.deployment("credentials", "secret-sync",
				"registry.example.com/secret-sync:2", 0)
			c.list(sync.objects())
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, []string{"n1"})
			c.after(10 * time.Minute)
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					if round == 1 {
						*w.deployment.Spec.Replicas = 2
						w.setReady(1)
						c.update(w.objects())
					}
					pod := w.pod(1, "n1", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedMount", "MountVolume.SetUp "+
						"failed for volume \"creds\" : secret \""+
						pod.Name+"-creds\" not found", round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap4bStatefulSetRecreate: a StatefulSet is deleted with its pods kept
// and created again to change an immutable field; every pod restarts
// within minutes, ready before it has loaded its data, and the clients
// that reconnect together OOM-kill a downstream service.
func gap4bStatefulSetRecreate() scenario {
	return scenario{
		expect: g2Exp("statefulset-recreated-restarts-all-pods",
			"A StatefulSet is recreated and restarts every pod at once; "+
				"the clients that reconnect together exhaust the memory "+
				"of a downstream Deployment, which is OOMKilled. The "+
				"reconnects are not visible, so the Deployment is the "+
				"root.",
			"deployment/shop/api", "notify", "node//n1",
			"node//n2").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			api := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			api.rollout(lifecycleMemoryLimit("256Mi"))
			api.setReady(2)
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(10 * time.Minute)
			recreated := sts.DeepCopy()
			recreated.UID = "uid-recreated"
			recreated.Generation, recreated.Status.ObservedGeneration = 1, 1
			c.remove(sts)
			c.create(recreated)
			for i := range 3 {
				node := "n" + itoa(i%2+1)
				c.update(operatorBroker(c, recreated, i, node, startedNow,
					notReady))
				c.after(20 * time.Second)
				c.update(operatorBroker(c, recreated, i, node, startedNow))
			}
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(api.pod(0, "n1", crashLoop(137, "OOMKilled", "",
					restarts)), api.pod(1, "n2", crashLoop(137, "OOMKilled",
					"", restarts)))
				api.setReady(0)
				c.update(api.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap4bSkewedLoad: clients keep one long-lived connection each and all of
// them reach the same replica; it is OOM-killed again and again while its
// two siblings idle.
func gap4bSkewedLoad() scenario {
	return scenario{
		expect: g2Exp("one-replica-takes-all-connections-oom",
			"One replica of a three-replica service is OOMKilled "+
				"repeatedly and goes unready while its siblings are "+
				"fine and no node reports a fault.",
			"deployment/shop/api", "notify", "node//n1", "node//n2",
			"node//n3").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 3)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"), w.pod(2, "n3"))
			c.after(5 * time.Minute)
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(137, "OOMKilled", "",
					restarts)))
				w.setReady(2)
				c.update(w.objects())
				c.after(2 * time.Minute)
			}
		},
	}
}

// gap4bCronFlood: a CronJob scheduled every minute never cleans its Jobs
// in a namespace without a pod quota; the objects pile up until the API
// server is OOM-killed.
func gap4bCronFlood() scenario {
	return scenario{
		expect: g2Exp("cronjob-every-minute-fills-api-server-memory",
			"A CronJob that runs every minute and keeps every Job "+
				"piles up objects until the API server is OOMKilled "+
				"and its health probe fails.",
			"cronjob/reports/minutely-sync", "page", "node//n1",
			"apiserver//kube-apiserver").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(staticPod(c, "kube-apiserver", "n1",
				"registry.k8s.io/kube-apiserver:v1.30.2"))
			cron := scheduleCronJob(c, "reports", "minutely-sync",
				"* * * * *")
			keep := int32(1000000)
			cron.Spec.SuccessfulJobsHistoryLimit = &keep
			cron.Spec.FailedJobsHistoryLimit = &keep
			c.list(cron)
			c.probe(kube.APIServer, "")
			for i := range 30 {
				c.after(time.Minute)
				job := finishedJob(c, cron, fmt.Sprintf("minutely-sync-%d",
					2925000+i), c.now, 20*time.Second)
				c.create(job)
				ran := metav1.NewTime(c.now)
				cron = cron.DeepCopy()
				cron.Status.LastScheduleTime = &ran
				cron.Status.LastSuccessfulTime = &ran
				c.update(cron)
			}
			for restarts := int32(1); restarts <= 4; restarts++ {
				c.update(staticPod(c, "kube-apiserver", "n1",
					"registry.k8s.io/kube-apiserver:v1.30.2",
					crashLoop(137, "OOMKilled", "", restarts)))
				c.probe(kube.APIServer, "Get \"https://10.96.0.1:443/"+
					"readyz\": dial tcp 10.96.0.1:443: connect: "+
					"connection refused")
				c.after(time.Minute)
			}
		},
	}
}

// gap4bEtcdIntermittent: during provider maintenance a control-plane node
// loses its path to etcd for a minute at a time, three times; the API
// server times out and recovers each time.
func gap4bEtcdIntermittent() scenario {
	return scenario{
		expect: g2Exp("etcd-path-intermittent-during-maintenance",
			"etcd is unreachable for a minute at a time, three times in "+
				"fifteen minutes, with the API server timing out and "+
				"recovering after each.", "etcd//etcd", "notify",
			"node//n1", "node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			controlPlaneFleet(c)
			c.probe(kube.Etcd, "")
			c.probe(kube.APIServer, "")
			for range 3 {
				c.after(4 * time.Minute)
				for range 2 {
					c.probe(kube.Etcd, "[-]etcd failed: context deadline "+
						"exceeded")
					c.probe(kube.APIServer, "")
					c.after(30 * time.Second)
				}
				c.probe(kube.Etcd, "")
			}
		},
	}
}

// staticPod is a control-plane pod the kubelet of node runs from a
// manifest: a mirror pod in kube-system owned by its node, with a 2Gi
// memory limit.
func staticPod(
	c *cluster, component, node, image string, states ...podState,
) *corev1.Pod {
	yes := true
	name := component + "-" + c.n(node)
	meta := c.meta("kube-system", name)
	meta.UID = types.UID(name)
	meta.Labels = map[string]string{"component": component,
		"tier": "control-plane"}
	meta.Annotations = map[string]string{
		"kubernetes.io/config.mirror": "0123456789abcdef"}
	meta.CreationTimestamp = metav1.NewTime(c.start.Add(-time.Hour))
	meta.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1", Kind: "Node", Name: c.n(node),
		UID: types.UID(c.n(node)), Controller: &yes}}
	quantity := resource.MustParse("2Gi")
	pod := &corev1.Pod{ObjectMeta: meta, Spec: corev1.PodSpec{
		NodeName: c.n(node), Containers: []corev1.Container{{
			Name: component, Image: image,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: quantity},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: quantity}},
		}},
	}}
	running(c, pod)
	for _, state := range states {
		state(c, pod)
	}
	return pod
}
