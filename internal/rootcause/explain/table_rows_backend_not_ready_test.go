package explain

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// notReadyCache marks the cache pods as running but not ready, with no
// finding of their own.
func notReadyCache(f *fixture, cache []inventory.EntityID) {
	for _, pod := range cache {
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: pod, Attributes: map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Running"),
				kube.AttrReady: inventory.Bool(false),
			}})
	}
}

var backendNotReadyRowCases = []rowCase{
	{row: "called-service-backends-not-ready",
		want: "deployment/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			_, api, cache := redisFixture(f, 1, 0)
			notReadyCache(f, cache)
			failAPI(f, api, refusedRedis)
			return containerOf(api[0])
		}},
	// The pod is the cause; the incident lifts it to its workload.
	{row: "backends-not-ready", want: "pod/shop/redis-1-0",
		build: func(f *fixture) inventory.EntityID {
			svc, _, cache := redisFixture(f, 1, 0)
			notReadyCache(f, cache)
			f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
			return svc
		}},
}

// A Service whose backends run but are not ready only repeats their
// state: the workload is the cause, below high confidence while nothing
// says why its pods are not ready.
func TestExplainBlamesTheWorkloadBehindANotReadyCalledService(
	t *testing.T,
) {
	f := newFixture(t)
	svc, api, cache := redisFixture(f, 1, 0)
	notReadyCache(f, cache)
	f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
	failAPI(f, api, refusedRedis)

	c := requireCause(t, f.explain(), containerOf(api[0]),
		"deployment/shop/redis")
	assert.Less(t, c.Confidence, 0.8,
		"endpoint state alone is not proof enough for high confidence")
}
