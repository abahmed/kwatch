package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const refusedRedis = "dial tcp redis:6379: connect: connection refused"

// redisFixture is a cache Service with its pods and the API pods that
// call it by name. cachePods are the cache's pods; ready is how many
// endpoints the Service has ready.
func redisFixture(
	f *fixture, cachePods, ready int,
) (service inventory.EntityID, api, cache []inventory.EntityID) {
	service = inventory.CoreID(kube.KindService, "shop", "redis")
	slice := inventory.CoreID(kube.KindEndpointSlice, "shop", "redis-x")
	f.add(service)
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: slice,
		Attributes: map[string]inventory.Value{
			kube.AttrEndpoints:      inventory.Number(float64(cachePods)),
			kube.AttrEndpointsReady: inventory.Number(float64(ready)),
		}})
	f.relate(slice, inventory.Backs, service)
	if cachePods > 0 {
		cache = f.workload("shop", "redis", cachePods)
		for _, pod := range cache {
			f.links[service] = append(f.links[service],
				Link{Type: inventory.Selects, To: pod})
		}
	}
	api = f.workload("shop", "api", 2)
	return service, api, cache
}

var serviceCallRowCases = []rowCase{
	{row: "called-service-no-endpoints", want: "service/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			_, api, _ := redisFixture(f, 0, 0)
			failAPI(f, api, refusedRedis)
			return containerOf(api[0])
		}},
	{row: "called-service-backends-failing", want: "deployment/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			svc, api, cache := redisFixture(f, 1, 0)
			f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
			f.fail(cache[0], detection.ModeNotReady, failingH, 1, "")
			failAPI(f, api, refusedRedis)
			return containerOf(api[0])
		}},
	{row: "called-service-backend-changed", want: "deployment/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			svc, api, _ := redisFixture(f, 0, 0)
			cache := inventory.CoreID(kube.KindDeployment, "shop", "redis")
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: svc, Attributes: map[string]inventory.Value{
					kube.AttrSelector: inventory.Text("app=redis")}})
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: cache, Attributes: map[string]inventory.Value{
					kube.AttrTemplateLabels: inventory.Text("app=redis")}})
			f.change(cache, 5, "spec.replicas")
			failAPI(f, api, refusedRedis)
			return containerOf(api[0])
		}},
	{row: "policy-blocks-call", want: "networkpolicy/shop/deny-all",
		build: func(f *fixture) inventory.EntityID {
			api := policyFixture(f)
			f.change(addPolicy(f, "shop", "deny-all", denyEgress), 1)
			return containerOf(api[0])
		}},
}

func failAPI(f *fixture, api []inventory.EntityID, text string) {
	for _, pod := range api {
		f.failError(containerOf(pod), "CrashLoop", text)
	}
}

// requireRoot fails unless the cause of id is want.
func requireRoot(
	t *testing.T, f *fixture, id inventory.EntityID, want string,
) Cause {
	t.Helper()
	return requireCause(t, f.explain(), id, want)
}

func TestServiceCallRootsAtServiceWithNoEndpoints(t *testing.T) {
	f := newFixture(t)
	_, api, _ := redisFixture(f, 0, 0)
	failAPI(f, api, refusedRedis)

	c := requireRoot(t, f, containerOf(api[0]), "service/shop/redis")

	if c.Row != "called-service-no-endpoints" {
		t.Fatalf("row = %q", c.Row)
	}
}

func TestServiceCallRootsAtFailingBackendWorkload(t *testing.T) {
	f := newFixture(t)
	svc, api, cache := redisFixture(f, 1, 0)
	f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
	f.fail(containerOf(cache[0]), detection.ModeCrashLoop, failingH, 1, "")
	f.fail(inventory.CoreID(kube.KindDeployment, "shop", "redis"),
		detection.ModeUnavailable, failingH, 1, "")
	failAPI(f, api, refusedRedis)

	e := f.explain()

	for _, effect := range []inventory.EntityID{containerOf(api[0]),
		containerOf(api[1]), containerOf(cache[0])} {
		requireCause(t, e, effect, "deployment/shop/redis")
	}
	if got := causes(e); len(got) != 1 {
		t.Fatalf("causes = %v, want the cache workload alone", got)
	}
}

func TestServiceCallBlamesBackendScaledToZero(t *testing.T) {
	f := newFixture(t)
	svc, api, _ := redisFixture(f, 0, 0)
	cache := inventory.CoreID(kube.KindDeployment, "shop", "redis")
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: svc,
		Attributes: map[string]inventory.Value{
			kube.AttrSelector: inventory.Text("app=redis")}})
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: cache,
		Attributes: map[string]inventory.Value{
			kube.AttrTemplateLabels: inventory.Text("app=redis")}})
	f.change(cache, 5, "spec.replicas")
	failAPI(f, api, refusedRedis)

	c := requireRoot(t, f, containerOf(api[0]), "deployment/shop/redis")

	if c.Row != "called-service-backend-changed" {
		t.Fatalf("row = %q", c.Row)
	}
}

func TestServiceCallIgnoresServiceWithReadyEndpoints(t *testing.T) {
	f := newFixture(t)
	_, api, _ := redisFixture(f, 2, 2)
	failAPI(f, api, refusedRedis)

	c, ok := f.explain().CauseOf(containerOf(api[0]))

	if ok && c.Root.Kind == kube.KindService {
		t.Fatalf("blamed %v, which has ready endpoints", c.Root)
	}
}

func TestServiceCallNeedsTheErrorToNameTheService(t *testing.T) {
	f := newFixture(t)
	svc, api, _ := redisFixture(f, 0, 0)
	for _, pod := range api {
		f.relate(pod, inventory.Calls, svc)
	}
	failAPI(f, api, "FATAL: password authentication failed for user app")

	c, ok := f.explain().CauseOf(containerOf(api[0]))

	if ok && c.Root == svc {
		t.Fatalf("blamed %v for an error that does not name it", svc)
	}
}

func TestServiceCallIgnoresDNSServerFailure(t *testing.T) {
	f := newFixture(t)
	_, api, _ := redisFixture(f, 0, 0)
	failAPI(f, api, "dial tcp: lookup redis on 10.96.0.10:53: i/o timeout")

	c, ok := f.explain().CauseOf(containerOf(api[0]))

	if ok && c.Root.Kind == kube.KindService {
		t.Fatalf("blamed %v for a DNS outage", c.Root)
	}
}

func TestServiceInReadsHostsAfterWords(t *testing.T) {
	f := newFixture(t)
	redisFixture(f, 0, 0)
	pod := inventory.CoreID(kube.KindPod, "shop", "api-1-0")
	cases := map[string]bool{
		"Error 111 connecting to redis:6379. Connection refused.": true,
		"dial tcp redis.shop.svc.cluster.local:6379: connect: " +
			"connection refused": true,
		"redis: connection refused":                        false,
		"dial tcp cache:6379: connect: connection refused": false,
		"connection refused":                               false,
	}
	v := newView(f.snapshot())
	for text, want := range cases {
		if _, ok := v.serviceIn(text, pod); ok != want {
			t.Errorf("%q: found = %v, want %v", text, ok, want)
		}
	}
}
