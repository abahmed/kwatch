package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var sameCheck = detection.Evidence{
	Label: detection.EvidenceLivenessSameCheck, Value: "true"}

// cascadeFixture is the redis fixture whose API pods are killed by a
// liveness probe that runs their readiness check. The pods are
// configured to call Service redis, but nothing they print names it.
func cascadeFixture(
	f *fixture, cachePods, ready int, evidence ...detection.Evidence,
) (service inventory.EntityID, api, cache []inventory.EntityID) {
	service, api, cache = redisFixture(f, cachePods, ready)
	for _, pod := range api {
		f.relate(pod, inventory.Calls, service)
	}
	liveKill(f, api, evidence...)
	return service, api, cache
}

var cascadeRowCases = []rowCase{
	{row: "called-service-backends-failing", want: "deployment/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			svc, api, cache := cascadeFixture(f, 1, 0, sameCheck)
			f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
			f.fail(cache[0], detection.ModeNotReady, failingH, 1, "")
			return containerOf(api[0])
		}},
	{row: "called-service-no-endpoints", want: "service/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			_, api, _ := cascadeFixture(f, 0, 0, sameCheck)
			return containerOf(api[0])
		}},
}

// TestCascadeNeedsTheSameCheck: liveness kills of pods that are
// configured to call a failing Service are not blamed on it unless the
// probe runs the readiness check. Configuration alone proves nothing.
func TestCascadeNeedsTheSameCheck(t *testing.T) {
	f := newFixture(t)
	svc, api, cache := cascadeFixture(f, 1, 0)
	f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
	f.fail(cache[0], detection.ModeNotReady, failingH, 1, "")
	c, ok := f.explain().CauseOf(containerOf(api[0]))
	if ok && (c.Root.Kind == kube.KindService ||
		c.Root.Name == "redis") {
		t.Fatalf("blamed %s without the same-check evidence", c.Root)
	}
}

// TestCascadeJoinsSeveralWorkloads: the restarts of two workloads that
// both call the failing Service have one root.
func TestCascadeJoinsSeveralWorkloads(t *testing.T) {
	f := newFixture(t)
	svc, api, cache := cascadeFixture(f, 1, 0, sameCheck)
	worker := f.workload("shop", "worker", 3)
	for _, pod := range worker {
		f.relate(pod, inventory.Calls, svc)
	}
	liveKill(f, worker, sameCheck)
	f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
	f.fail(cache[0], detection.ModeNotReady, failingH, 1, "")
	e := f.explain()
	requireCause(t, e, containerOf(api[0]), "deployment/shop/redis")
	requireCause(t, e, containerOf(worker[0]), "deployment/shop/redis")
}
