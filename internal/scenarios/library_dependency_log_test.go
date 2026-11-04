package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dependencyLogScenarios are crashes whose only clue is a dependency the
// previous log says the program could not connect to.
func dependencyLogScenarios() []scenario {
	return []scenario{cacheUnreachableAtStart(), cacheRetriesHealthy()}
}

// appRedisLog is the previous log of a .NET host that died on start
// because it could not reach Redis: the exception, then its frames.
var appRedisLog = []string{
	"info: Microsoft.Hosting.Lifetime[0] Application is starting",
	redisConnectionLine,
	"   at StackExchange.Redis.ConnectionMultiplexer.Connect(String " +
		"configuration)",
	"   at Api.Cache.CacheClient..ctor(String connection)",
	"   at Program.<Main>$(String[] args) in /src/Program.cs:line 48",
}

// appProbe is what the kubelet says of each probe while nothing
// listens on the port.
func appProbe(kind string) string {
	return kind + " probe failed: Get \"http://10.244.0.10:80/api/" +
		"Health/" + map[string]string{"Readiness": "readiness",
		"Liveness": "liveness"}[kind] + "\": dial tcp 10.244.0.10:80: " +
		"connect: connection refused"
}

// cacheUnreachableAtStart: a single-replica .NET Deployment
// crashes at start (exit 139, an unhandled exception) because it cannot
// connect to Redis; while it is down its readiness probe is refused and
// its Service has no endpoint. It recovers for a few minutes between
// bouts, so the incident flaps. Its previous log names Redis.
func cacheUnreachableAtStart() scenario {
	return scenario{
		expect: expectation{
			Name: "cache-unreachable-at-start",
			Description: "A single-replica Deployment exits 139 at start " +
				"with an unhandled Redis connection exception in its " +
				"previous log, fails readiness while down, and flaps.",
			Root: "deployment/web/api", Tier: "notify",
			MaxMessages: 3, Tail: duration(10 * time.Minute),
			MustNotBlame: []string{"node//n1", "service/web/api"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := apiWorkload(c)
			pod := w.pod(0, "n1")
			c.list(pod, clusterService(c, "web", "api", 80),
				appSlice(c, pod, true))
			c.after(3 * time.Minute)
			appBouts(c, w, appRedisLog)
		},
	}
}

// cacheRetriesHealthy: the same app logs one refused Redis connection on
// start, retries and serves; it never restarts or fails a probe. A
// transient retry is no incident and names no cause.
func cacheRetriesHealthy() scenario {
	return scenario{
		expect: expectation{
			Name: "cache-retry-healthy-quiet",
			Description: "A Deployment logs a Redis connection retry " +
				"but never restarts, stays ready and keeps its endpoint.",
			Quiet: true, MaxMessages: 0, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := apiWorkload(c)
			pod := w.pod(0, "n1")
			c.list(pod, clusterService(c, "web", "api", 80),
				appSlice(c, pod, true))
			c.after(3 * time.Minute)
			id := kube.ContainerID(pod.Namespace, pod.Name, "app")
			c.emit(kube.CrashLogObservation(id, c.now, appRedisLog))
			c.after(5 * time.Minute)
		},
	}
}

// appProbeEvent is the kubelet's Unhealthy event about the app
// container's probe of kind.
func appProbeEvent(
	c *cluster, pod *corev1.Pod, kind string, count int32,
) *corev1.Event {
	ev := c.warningEvent(pod, "Pod", "Unhealthy", appProbe(kind),
		"kubelet", count)
	ev.Name += "." + kind
	ev.InvolvedObject.FieldPath = "spec.containers{app}"
	return ev
}

// apiWorkload is the single-replica Deployment with HTTP readiness and
// liveness probes on port 80.
func apiWorkload(c *cluster) *workload {
	w := c.deployment("web", "api", "registry.example.com/api:2.3",
		1)
	probe := func(path string) *corev1.Probe {
		return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: path,
				Port: intstr.FromInt(80)}}, PeriodSeconds: 10}
	}
	for _, spec := range []*corev1.PodSpec{&w.deployment.Spec.Template.Spec,
		&w.replicaSet.Spec.Template.Spec} {
		spec.Containers[0].ReadinessProbe = probe("/api/Health/readiness")
		spec.Containers[0].LivenessProbe = probe("/api/Health/liveness")
		spec.Containers[0].Ports = []corev1.ContainerPort{
			{ContainerPort: 80}}
	}
	c.list(w.objects())
	return w
}

// appSlice is the Service's EndpointSlice with the pod as its one
// endpoint, ready or not.
func appSlice(
	c *cluster, pod *corev1.Pod, ready bool,
) *discoveryv1.EndpointSlice {
	slice := clusterSlice(c, "web", "api", pod)
	port := int32(80)
	slice.Ports = []discoveryv1.EndpointPort{{Port: &port}}
	slice.Endpoints[0].Conditions.Ready = boolPtr(ready)
	return slice
}

// appBouts runs three bouts of the start-up crash, each followed by a
// few minutes of serving, and ends down.
func appBouts(c *cluster, w *workload, log []string) {
	restarts, probes := int32(0), int32(0)
	for bout := range 3 {
		for range 2 {
			restarts++
			probes += 4
			pod := w.pod(0, "n1", crashLoop(139, "Error", "", restarts))
			c.update(pod)
			c.emit(kube.CrashLogObservation(kube.ContainerID(
				pod.Namespace, pod.Name, "app"), c.now, log))
			c.update(appSlice(c, pod, false))
			w.setReady(0)
			c.update(w.objects())
			c.after(30 * time.Second)
			unready := w.pod(0, "n1", restartedUnready(restarts))
			c.update(unready)
			c.after(40 * time.Second)
			c.warn(appProbeEvent(c, unready, "Readiness", probes))
			c.warn(appProbeEvent(c, unready, "Liveness", probes/2))
			c.after(20 * time.Second)
		}
		if bout == 2 {
			break
		}
		pod := w.pod(0, "n1", restartedReady(restarts))
		c.update(pod, appSlice(c, pod, true))
		w.setReady(1)
		c.update(w.objects())
		c.after(5 * time.Minute)
	}
}

// appTermination is the run that ended before the restart.
func appTermination(c *cluster) corev1.ContainerState {
	return corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
		ExitCode: 139, Reason: "Error",
		StartedAt:  metav1.NewTime(c.now.Add(-40 * time.Second)),
		FinishedAt: metav1.NewTime(c.now.Add(-30 * time.Second)),
	}}
}

// restartedUnready is a container that restarted after exit 139 and runs
// without listening yet: not ready.
func restartedUnready(restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		startedNow(c, pod)
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.LastTerminationState = appTermination(c)
	}
}

// restartedReady is a container that restarted after exit 139 and now
// serves.
func restartedReady(restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		startedNow(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.LastTerminationState = appTermination(c)
	}
}
