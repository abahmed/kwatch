package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dependencyRemovedScenarios are shared dependencies removed on
// purpose, every workload that uses them losing readiness at once, and
// the removal nobody notices.
func dependencyRemovedScenarios() []scenario {
	return []scenario{sharedDependencyRemovedOvernight(),
		sharedDependencyRemovedUnused()}
}

// removedCache is the shared cache a scheduled job deletes: Service and
// pods of namespace data.
const removedCache = "cache"

// removedCallers is how many workloads use the cache.
const removedCallers = 27

// sharedDependencyRemovedOvernight: every evening a scheduled job
// deletes the shared cache (its Service, Deployment and pod) and then
// scales a node down. The 27 workloads that use it read its address
// from a Secret, stay up but fail readiness within three minutes, each
// logging its failed call in its own client's words, all naming the
// cache. Five of them also name in their configuration a Service that
// never existed and that nothing calls. One more workload crashes on a
// setting of its own at the same time. Next morning the cache is back
// and everyone recovers. One incident for the cache with one recovery,
// one of its own for the crash.
func sharedDependencyRemovedOvernight() scenario {
	return scenario{
		expect: expectation{
			Name: "shared-dependency-removed-overnight",
			Description: "A scheduled job deletes the shared cache " +
				"Service and its pods in the evening; 27 workloads " +
				"that use it fail readiness within three minutes, " +
				"one more crashes on its own setting; the cache " +
				"returns next morning.",
			Root:       "service/data/" + removedCache,
			OtherRoots: []string{"deployment/shop/reports"},
			Tier:       "notify", MaxMessages: 4, MaxPages: 1,
			MustNotBlame: []string{"service/shop/legacy-search",
				"node//n1", "node//n2", "node//n3"},
			Alone: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) { buildDependencyRemoved(c, true) },
	}
}

// sharedDependencyRemovedUnused: the same evening job deletes the
// cache, but no workload calls it at night: all stay ready. Nothing is
// said.
func sharedDependencyRemovedUnused() scenario {
	return scenario{
		expect: expectation{
			Name: "shared-dependency-removed-unused",
			Description: "A scheduled job deletes the shared cache " +
				"Service and its pods in the evening; the workloads " +
				"configured with it stay ready all night.",
			Quiet: true, Alone: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) { buildDependencyRemoved(c, false) },
	}
}

// removedCallerName is the name of caller i.
func removedCallerName(i int) string { return fmt.Sprintf("svc-%02d", i) }

// cacheHost is the cache's full cluster name.
func cacheHost(c *cluster) string {
	return c.n(removedCache) + ".data.svc.cluster.local"
}

// removedLine is how caller i logs its failed call: three client
// libraries, each naming the cache its own way.
func removedLine(c *cluster, i int) string {
	short := c.n(removedCache) + ".data"
	switch i % 3 {
	case 0:
		return "StackExchange.Redis.RedisConnectionException: It was " +
			"not possible to connect to the redis server(s). " +
			"UnableToConnect on " + short + ":6379/Interactive"
	case 1:
		return "Error: Redis connection to " + short + ":6379 failed - " +
			"getaddrinfo ENOTFOUND " + short
	default:
		return "level=error msg=\"cache ping failed\" err=\"dial " +
			"tcp: lookup " + cacheHost(c) + " on 10.96.0.10:53: no " +
			"such host\""
	}
}

// removedNamespaces are the callers' namespaces.
var removedNamespaces = []string{"shop", "billing", "fleet"}

// buildDependencyRemoved runs the evening, the night and the morning;
// with failing the callers and reports fail through the night.
func buildDependencyRemoved(c *cluster, failing bool) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
		c.node("n3", "zone-a"))
	for _, ns := range append([]string{"data"}, removedNamespaces...) {
		c.list(&corev1.Namespace{ObjectMeta: c.meta("", ns)})
	}
	cache, objects := removedCacheObjects(c)
	c.list(objects...)
	callers := removedCallerFleet(c)
	reports := c.deployment("shop", "reports",
		"registry.example.com/reports:3", 1)
	c.list(reports.objects())
	c.list(reports.pod(0, "n2"))
	c.after(15 * time.Minute)
	c.remove(objects...)
	if failing {
		removedEvening(c, callers, reports)
	}
	c.after(10 * time.Minute)
	c.remove(last(c, c.node("n3", "zone-a")))
	c.after(10 * time.Hour)
	removedMorning(c, cache, callers, reports)
}

// removedEvening fails the callers within three minutes of the
// removal, and reports on its own setting.
func removedEvening(c *cluster, callers []*workload, reports *workload) {
	began := c.now
	for round := range 9 {
		c.after(20 * time.Second)
		for i, w := range callers {
			if i%9 <= round {
				removedCallerFails(c, w, i, began, int32(round+1))
			}
		}
		c.update(reports.pod(0, "n2", crashLoop(1, "Error",
			"fatal: unknown setting \"export.mode\" in "+
				"/etc/reports.yaml", int32(round+1))))
		reports.setReady(0)
		c.update(reports.objects())
	}
}

// removedCacheObjects is the cache: Service, slice, Deployment and pod.
func removedCacheObjects(c *cluster) (*workload, []runtime.Object) {
	cache := c.deployment("data", removedCache,
		"registry.example.com/redis:7", 1)
	pod := cache.pod(0, "n3")
	service := clusterService(c, "data", removedCache, 6379)
	slice := clusterSlice(c, "data", removedCache, pod)
	d, rs := cache.objects()
	return cache, []runtime.Object{d, rs, pod, service, slice}
}

// removedCallerFleet lists the callers, one replica each on n1 or n2.
// Their configuration names the cache's port only: the address is in a
// Secret. Five also name a Service that never existed.
func removedCallerFleet(c *cluster) []*workload {
	var fleet []*workload
	for i := range removedCallers {
		name := removedCallerName(i)
		w := c.deployment(removedNamespaces[i%3], name,
			"registry.example.com/"+name+":1", 1)
		legacy := i < 5
		configTemplate(w, func(spec *corev1.PodSpec) {
			spec.Containers[0].Env = []corev1.EnvVar{{
				Name: "CACHE_PORT", Value: "6379"}}
			if legacy {
				spec.Containers[0].Env = append(spec.Containers[0].Env,
					corev1.EnvVar{Name: "SEARCH_URL", Value: "http://" +
						c.n("legacy-search") + ".shop.svc:9200"})
			}
		})
		c.list(w.objects())
		c.list(w.pod(0, removedCallerNode(i)))
		fleet = append(fleet, w)
	}
	return fleet
}

func removedCallerNode(i int) string { return []string{"n1", "n2"}[i%2] }

// removedCallerFails marks caller i running but not ready since began,
// with count failed readiness probes and its failed call in its log.
func removedCallerFails(
	c *cluster, w *workload, i int, began time.Time, count int32,
) {
	pod := w.pod(0, removedCallerNode(i), notReadySince(began))
	c.update(pod)
	ev := c.warningEvent(pod, "Pod", "Unhealthy", "Readiness probe "+
		"failed: HTTP probe failed with statuscode: 503", "kubelet", count)
	ev.InvolvedObject.FieldPath = "spec.containers{app}"
	c.warn(ev)
	c.emit(kube.ProbeLogObservation(containerOf(pod), c.now,
		[]string{"info: GET /health/ready 503", removedLine(c, i)}))
	w.setReady(0)
	c.update(w.objects())
}

// removedMorning brings n3 and the cache back, then every caller.
func removedMorning(
	c *cluster, cache *workload, callers []*workload, reports *workload,
) {
	n3 := c.node("n3", "zone-a")
	n3.CreationTimestamp = metav1.NewTime(c.now)
	c.create(n3)
	c.after(2 * time.Minute)
	pod := cache.pod(1, "n3")
	cache.setReady(1)
	d, rs := cache.objects()
	c.create(d, rs, pod, clusterService(c, "data", removedCache, 6379),
		clusterSlice(c, "data", removedCache, pod))
	c.after(time.Minute)
	for i, w := range callers {
		c.update(w.pod(0, removedCallerNode(i)))
		w.setReady(1)
		c.update(w.objects())
	}
	c.update(reports.pod(0, "n2"))
	reports.setReady(1)
	c.update(reports.objects())
}
