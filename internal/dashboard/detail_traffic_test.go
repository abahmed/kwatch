package dashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// slice adds an EndpointSlice backing svc with total endpoints, ready
// of them ready.
func (tm *testModel) slice(svc inventory.EntityID, name string,
	total, ready int,
) {
	id := inventory.CoreID(kube.KindEndpointSlice, svc.Namespace, name)
	tm.observe(id, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(float64(total)),
		kube.AttrEndpointsReady: inventory.Number(float64(ready)),
	})
	tm.relate(id, inventory.Backs, svc)
}

func TestDetailTrafficCountsReadyEndpoints(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	payments := inventory.CoreID(kube.KindService, "shop", "payments")
	healthy := inventory.CoreID(kube.KindService, "shop", "web")
	empty := inventory.CoreID(kube.KindService, "shop", "empty")
	ing := inventory.CoreID(kube.KindIngress, "shop", "front")
	for _, id := range []inventory.EntityID{payments, healthy, empty, ing} {
		tm.observe(id, nil)
	}
	tm.relate(payments, inventory.Selects, pod)
	tm.slice(payments, "payments-a", 1, 0)
	tm.slice(payments, "payments-b", 1, 0)
	tm.slice(healthy, "web-a", 3, 3)
	tm.relate(ing, inventory.RoutesTo, payments)
	p := failingAt("t", root, testEpoch.Add(-time.Hour))
	p.Impact = []inventory.EntityID{healthy, empty}

	d := detailOf(t, contextServer(tm.m, false, p), "t")
	want := []TrafficRow{
		{Kind: "service", Namespace: "shop", Name: "empty"},
		{Kind: "service", Namespace: "shop", Name: "payments", Total: 2},
		{Kind: "ingress", Namespace: "shop", Name: "front", Total: 2,
			Backends: []string{"payments"}},
		{Kind: "service", Namespace: "shop", Name: "web", Ready: 3,
			Total: 3},
	}
	if fmt.Sprint(d.Traffic) != fmt.Sprint(want) {
		t.Fatalf("traffic = %+v\nwant %+v", d.Traffic, want)
	}
}

func TestDetailTrafficIsBoundedAndHiddenWithoutServices(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	p := failingAt("t", root, testEpoch.Add(-time.Hour))
	if d := detailOf(t, contextServer(tm.m, false, p), "t"); d.Traffic != nil {
		t.Fatalf("traffic without Services = %+v", d.Traffic)
	}
	for i := range maxTraffic + 3 {
		svc := inventory.CoreID(kube.KindService, "shop",
			fmt.Sprintf("s%02d", i))
		tm.observe(svc, nil)
		tm.relate(svc, inventory.Selects, pod)
		tm.slice(svc, fmt.Sprintf("s%02d-a", i), 1, 1)
	}
	d := detailOf(t, contextServer(tm.m, false, p), "t")
	if len(d.Traffic) != maxTraffic || d.Traffic[0].Name != "s00" ||
		d.Truncated == nil || d.Truncated.Traffic != 3 {
		t.Fatalf("traffic = %+v, truncated %+v", d.Traffic, d.Truncated)
	}
}

func TestDetailTrafficSkipsServicesWithoutKnownSlices(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	svc := inventory.CoreID(kube.KindService, "shop", "payments")
	tm.observe(svc, nil)
	tm.relate(svc, inventory.Selects, pod)
	d := detailOf(t, contextServer(tm.m, false,
		failingAt("t", root, testEpoch.Add(-time.Hour))), "t")
	if d.Traffic != nil {
		t.Fatalf("no slice is known, yet traffic = %+v", d.Traffic)
	}
}
