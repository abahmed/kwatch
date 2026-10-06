package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// seen is an observation at a time, from the test source.
func seen(
	id inventory.EntityID, at time.Time, attrs map[string]inventory.Value,
) inventory.Observation {
	o := observed(id, attrs)
	o.Source, o.At = "test", at
	return o
}

// link is a relation from the test source.
func link(
	from inventory.EntityID, relation inventory.RelationType,
	to inventory.EntityID,
) inventory.Observation {
	o := related(from, relation, to)
	o.Source = "test"
	return o
}

func TestEngineMarksTheServiceOfAnUpdatedEndpointSlice(t *testing.T) {
	e := solveInputEngine(t)
	service := inventory.CoreID(kube.KindService, "shop", "web")
	slice := inventory.CoreID(kube.KindEndpointSlice, "shop", "web-1")
	e.apply([]inventory.Observation{link(slice, inventory.Backs,
		service)})
	dirty := e.apply([]inventory.Observation{seen(slice, time.Now(),
		map[string]inventory.Value{
			kube.AttrEndpointsReady: inventory.Number(0),
		})})
	if !containsEntity(dirty, service) {
		t.Fatalf("dirty = %v, want the backed Service", dirty)
	}
}

func TestEngineMarksReferrersOfAGoneObject(t *testing.T) {
	e := solveInputEngine(t)
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	config := inventory.CoreID(kube.KindConfigMap, "shop", "settings")
	ingress := inventory.CoreID(kube.KindIngress, "shop", "front")
	service := inventory.CoreID(kube.KindService, "shop", "web")
	e.apply([]inventory.Observation{
		seen(config, time.Now(), nil),
		seen(service, time.Now(), nil),
		link(web, inventory.References, config),
		link(ingress, inventory.RoutesTo, service),
	})
	cases := map[inventory.EntityID]inventory.EntityID{
		config: web, service: ingress,
	}
	for gone, referrer := range cases {
		dirty := e.apply([]inventory.Observation{{Kind: inventory.Gone,
			Source: "test", At: time.Now(), Entity: gone}})
		if !containsEntity(dirty, referrer) {
			t.Fatalf("%s gone: dirty = %v, want %s", gone, dirty,
				referrer)
		}
	}
}

// Endpoints going unready reach the Service's detector at once, so the
// finding is raised as soon as its waiting period ends, not whenever
// something else happens to touch the Service.
func TestEngineRaisesServiceNoEndpointsOnSliceUpdate(t *testing.T) {
	start := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	service := inventory.CoreID(kube.KindService, "shop", "web")
	slice := inventory.CoreID(kube.KindEndpointSlice, "shop", "web-1")
	endpoints := func(ready float64) map[string]inventory.Value {
		return map[string]inventory.Value{
			kube.AttrEndpoints:      inventory.Number(2),
			kube.AttrEndpointsReady: inventory.Number(ready),
		}
	}
	h.engine.Submit(context.Background(),
		seen(service, start, map[string]inventory.Value{
			kube.AttrServiceType: inventory.Text("ClusterIP"),
			kube.AttrSelector:    inventory.Text("app=web"),
		}),
		seen(slice, start, endpoints(2)),
		link(slice, inventory.Backs, service))
	h.engine.step(context.Background(), h.now, h.checks)

	unready := start.Add(time.Minute)
	h.now = unready
	h.engine.Submit(context.Background(),
		seen(slice, unready, endpoints(0)))
	h.engine.step(context.Background(), h.now, h.checks)

	h.now = unready.Add(detectors.DefaultNoEndpoints)
	h.engine.step(context.Background(), h.now, h.checks)
	for _, f := range h.engine.tracker.Active(service) {
		if f.Reason == reasons.ServiceNoEndpoints {
			return
		}
	}
	t.Fatalf("ServiceNoEndpoints not raised: %v",
		h.engine.tracker.Active(service))
}

func containsEntity(ids []inventory.EntityID, id inventory.EntityID) bool {
	for _, got := range ids {
		if got == id {
			return true
		}
	}
	return false
}

// waitClock records the delay of every timer it hands out.
type waitClock struct {
	now   time.Time
	waits []time.Duration
}

func (c *waitClock) Now() time.Time { return c.now }

func (c *waitClock) After(d time.Duration) <-chan time.Time {
	c.waits = append(c.waits, d)
	return nil
}

func TestEngineTimerDoesNotWaitWithDirtyEntities(t *testing.T) {
	clock := &waitClock{now: time.Now()}
	e := newTestEngine(t, &fakeClock{now: clock.now}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Clock = clock })
	e.timer(newRechecks(), time.Time{})
	e.dirty = []inventory.EntityID{
		inventory.CoreID(kube.KindService, "shop", "web")}
	e.timer(newRechecks(), time.Time{})
	if len(clock.waits) != 2 || clock.waits[0] != heartbeat ||
		clock.waits[1] != 0 {
		t.Fatalf("waits = %v, want the heartbeat then none", clock.waits)
	}
}

// Creating a missing referenced object re-judges whoever referenced it
// in the same loop: the Ingress finding about its TLS Secret clears as
// soon as the Secret exists.
func TestEngineMarksReferrersOfACreatedObject(t *testing.T) {
	start := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	ingress := inventory.CoreID(kube.KindIngress, "shop", "front")
	secret := inventory.CoreID(kube.KindSecret, "shop", "front-tls")
	missing := func() bool {
		for _, f := range h.engine.tracker.Active(ingress) {
			if f.Reason == reasons.IngressTLSSecretMissing {
				return true
			}
		}
		return false
	}
	// A new Ingress may wait for its TLS Secret (cert-manager makes it);
	// the finding exists only once the Ingress is older than that grace.
	old := start.Add(-detectors.DefaultTLSSecretGrace - time.Minute)
	h.engine.Submit(context.Background(),
		seen(ingress, old, nil),
		link(ingress, inventory.References, secret))
	h.engine.step(context.Background(), h.now, h.checks)
	if !missing() {
		t.Fatalf("TLS Secret finding not raised: %v",
			h.engine.tracker.Active(ingress))
	}

	h.now = start.Add(time.Minute)
	h.engine.Submit(context.Background(), seen(secret, h.now, nil))
	h.engine.step(context.Background(), h.now, h.checks)
	if missing() {
		t.Fatal("TLS Secret finding still active after one loop")
	}
}
