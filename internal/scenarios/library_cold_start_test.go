package scenarios

import (
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// coldStartScenarios are failures of a whole cluster coming up at once.
func coldStartScenarios() []scenario {
	return []scenario{coldStartSharedStartError()}
}

// cacheStartLine is the first error line each service logs when the
// cache it needs at start-up is not reachable yet.
const cacheStartLine = "StackExchange.Redis.RedisConnectionException: " +
	"It was not possible to connect to the redis server(s). Error " +
	"connecting right now."

// coldStartApps are the services of the cold start, one replica each.
var coldStartApps = []string{"shop/cart", "shop/orders", "shop/search",
	"billing/ledger", "billing/invoicer", "media/thumbs"}

// coldStartSharedStartError: every node is new and every pod starts at
// once. Six services cannot reach their cache at start-up and log the
// same first error line, but each fails its own way: two crash-loop,
// two are killed by their liveness probe (their preStop hook fails as
// they die), one exits with a segfault and one never becomes ready.
// GitOps touches a ConfigMap meanwhile and Karpenter later consolidates
// a node. The shared start-up error is the one root.
func coldStartSharedStartError() scenario {
	notBlamed := []string{"node//n0", "node//n1", "node//n2", "node//n3",
		"configmap/shop/settings"}
	for _, app := range coldStartApps {
		notBlamed = append(notBlamed, "deployment/"+app)
	}
	return scenario{
		expect: expectation{
			Name: "cold-start-shared-start-error",
			Description: "On new nodes six services fail at start-up " +
				"in different ways (crash loop, liveness kill, " +
				"segfault, not ready) with the same first error line.",
			Root: "failure-signature//stackexchange.redis." +
				"redisconnectionexception: it was not possible to " +
				"connect to",
			Tier: "notify", MaxMessages: 3, MustNotBlame: notBlamed,
			Tail: duration(10 * time.Minute),
		},
		build: buildColdStart,
	}
}

func buildColdStart(c *cluster) {
	c.list(c.node("n0", "zone-a"), poolNode(c, "n1", c.now),
		poolNode(c, "n2", c.now))
	settings := configMap(c, "shop", "settings",
		map[string]string{"mode": "a"})
	c.list(settings)
	var fleet []*workload
	for _, app := range coldStartApps {
		fleet = append(fleet, coldStartApp(c, app))
	}
	c.after(time.Minute)
	for restarts := int32(1); restarts <= 4; restarts++ {
		for i, w := range fleet {
			coldStartFail(c, w, i, restarts)
		}
		if restarts == 2 {
			gitOpsTouch(c, settings)
		}
		c.after(80 * time.Second)
	}
	for i, w := range fleet {
		c.update(w.pod(0, coldStartNode(i), startedNow))
		w.setReady(1)
		c.update(w.objects())
	}
	c.after(8 * time.Minute)
	consolidate(c, fleet)
	c.after(10 * time.Minute)
}

// coldStartApp lists a one-replica Deployment with HTTP probes whose
// pod is pending: the cluster is still coming up.
func coldStartApp(c *cluster, app string) *workload {
	namespace, name, _ := strings.Cut(app, "/")
	w := c.deployment(namespace, name,
		"registry.example.com/"+name+":1.0", 1)
	configTemplate(w, func(spec *corev1.PodSpec) {
		app := &spec.Containers[0]
		app.Ports = []corev1.ContainerPort{{ContainerPort: 8080}}
		app.ReadinessProbe = httpProbe("/ready")
		app.LivenessProbe = httpProbe("/live")
	})
	w.setReady(0)
	c.list(w.objects())
	c.list(w.pod(0, "", pendingUnscheduled("0/0 nodes are available")))
	return w
}

func httpProbe(path string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: path, Port: intstr.FromInt32(8080)}},
		PeriodSeconds: 10, FailureThreshold: 3,
	}
}

// coldStartNode is where service i runs: the new nodes, but the last
// one lands on n0, the one node that survived the shutdown.
func coldStartNode(i int) string {
	if i == len(coldStartApps)-1 {
		return "n0"
	}
	return []string{"n1", "n2"}[i%2]
}

// coldStartFail moves service i one restart further in its own failure
// mode, and records the first error line of its previous log.
func coldStartFail(c *cluster, w *workload, i int, restarts int32) {
	node := coldStartNode(i)
	var state podState
	switch i {
	case 0, 1:
		state = crashLoop(1, "Error", "", restarts)
	case 2, 3:
		state = liveKilled(restarts, 70*time.Second)
	case 4:
		state = exitedOnOwn(139, restarts)
	default:
		state = exitedOnOwn(143, 1)
	}
	pod := w.pod(0, node, createdAt(c.start), state)
	if i == 5 {
		notReadySince(c.start)(c, pod)
	}
	c.update(pod)
	if i == 2 || i == 3 {
		livenessKill(c, pod)
	}
	c.emit(kube.CrashLogObservation(
		kube.ContainerID(pod.Namespace, pod.Name, "app"), c.now,
		[]string{"info: starting", cacheStartLine}))
	w.setReady(0)
	c.update(w.objects())
}

// exitedOnOwn is a container running again, not ready, after its last
// run exited with code by itself a moment ago.
func exitedOnOwn(code, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		liveKilled(restarts, 30*time.Second)(c, pod)
		last := pod.Status.ContainerStatuses[0].LastTerminationState
		last.Terminated.ExitCode = code
	}
}

// livenessKill records the kubelet's events of a liveness kill: the
// failed probe, then the preStop hook that fails as the container dies.
func livenessKill(c *cluster, pod *corev1.Pod) {
	probe := "Liveness probe failed: Get \"http://10.244.1.7:8080/live\": " +
		"dial tcp 10.244.1.7:8080: connect: connection refused"
	for _, ev := range []*corev1.Event{
		c.warningEvent(pod, "Pod", "Unhealthy", probe, "kubelet", 3),
		c.warningEvent(pod, "Pod", "FailedPreStopHook",
			"PreStopHook failed", "kubelet", 1),
	} {
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
	}
}

// gitOpsTouch is the GitOps controller syncing a ConfigMap of the
// namespace while the services fail: a change, but not the cause.
func gitOpsTouch(c *cluster, cm *corev1.ConfigMap) {
	edited := last(c, cm)
	edited.Data["mode"] = "b"
	at := metav1.NewTime(c.now)
	edited.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager:   "argocd-application-controller",
		Operation: metav1.ManagedFieldsOperationUpdate, Time: &at,
	}}
	c.update(edited)
}

// consolidate is Karpenter replacing node n1 after the recovery: it
// taints the node, its pods are evicted and come back healthy on n3.
func consolidate(c *cluster, fleet []*workload) {
	old := last(c, poolNode(c, "n1", c.start))
	old.Spec.Taints = []corev1.Taint{{Key: "karpenter.sh/disrupted",
		Effect: corev1.TaintEffectNoSchedule}}
	c.update(old)
	c.create(poolNode(c, "n3", c.now))
	c.after(20 * time.Second)
	for i, w := range fleet {
		if coldStartNode(i) != "n1" {
			continue
		}
		c.remove(w.pod(0, "n1"))
		c.create(w.pod(1, "n3", startedNow))
	}
	c.after(20 * time.Second)
	c.remove(old)
}
