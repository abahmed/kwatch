package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// waitingInit adds an init container to pod that is configured to wait
// for the redis Service and has run far longer than usual.
func waitingInit(f *fixture, pod inventory.EntityID) inventory.EntityID {
	init := inventory.CoreID(kube.KindContainer, pod.Namespace,
		pod.Name+"/wait-for-redis")
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: init,
		Attributes: map[string]inventory.Value{
			kube.AttrInit:         inventory.Bool(true),
			kube.AttrServiceCalls: inventory.Text("shop/redis:6379"),
		}})
	f.relate(init, inventory.PartOf, pod)
	f.fail(init, detection.ModeInitWaiting, failingH, 3, "")
	return init
}

var initWaitRowCases = []rowCase{
	{row: "init-waits-on-service", want: "service/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			svc, api, _ := redisFixture(f, 0, 0)
			f.fail(svc, detection.ModeNoEndpoints, failingH, 2, "")
			return waitingInit(f, api[0])
		}},
}

// An init container whose Service answers is not blamed on it.
func TestInitWaitIgnoresAHealthyService(t *testing.T) {
	f := newFixture(t)
	_, api, _ := redisFixture(f, 0, 0)
	init := waitingInit(f, api[0])

	c, ok := f.explain().CauseOf(init)

	if ok && c.Root.Kind == kube.KindService {
		t.Fatalf("blamed %s, which has no finding", c.Root)
	}
}
