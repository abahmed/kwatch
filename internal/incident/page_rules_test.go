package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// One critical pod behind an Ingress pages only when its Service lost
// the traffic: no ready backends, or most of them failing.
func TestManagerTrafficLostPagesOnlyWhenBackendsAreLost(t *testing.T) {
	cases := []struct {
		name         string
		total, ready float64
		want         Tier
	}{
		{"one of four failing", 4, 3, Notify},
		{"half failing", 4, 2, Notify},
		{"most failing", 4, 1, Page},
		{"none ready", 2, 0, Page},
		{"service gone", -1, 0, Page},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, Config{})
			pod, _ := workload(r, "web")
			slice := entity(kube.KindEndpointSlice, "web-x")
			svc := entity(kube.KindService, "web")
			r.relate(slice, inventory.RoutesTo, pod)
			r.relate(slice, inventory.Backs, svc)
			r.relate(entity(kube.KindIngress, "web"),
				inventory.RoutesTo, svc)
			if tc.total >= 0 {
				r.observe(svc)
			}
			_, err := r.model.Apply(inventory.Observation{
				Kind: inventory.Observed, Source: "test", At: t0,
				Entity: slice, Attributes: map[string]inventory.Value{
					kube.AttrEndpoints:      inventory.Number(tc.total),
					kube.AttrEndpointsReady: inventory.Number(tc.ready),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			r.raise(at(0), sig(pod, reasons.CrashLoopBackOff,
				detection.Critical))
			if got := r.only().Tier; got != tc.want {
				t.Fatalf("tier = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestManagerTrafficLostIgnoresServicesNobodyRoutesTo(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	slice := entity(kube.KindEndpointSlice, "web-x")
	r.relate(slice, inventory.RoutesTo, pod)
	r.relate(slice, inventory.Backs, entity(kube.KindService, "web"))
	r.observe(entity(kube.KindService, "web"))
	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Critical))
	if got := r.only().Tier; got != Notify {
		t.Fatalf("tier = %v, want notify without an Ingress", got)
	}
}
