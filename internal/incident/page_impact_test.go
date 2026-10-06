package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// impactRig is the shop Deployment "web" with one crashing pod, a
// Service selecting it, and the Service's EndpointSlice.
type impactRig struct {
	*rig
	pod, deploy, service, slice inventory.EntityID
}

// newImpactRig builds web with desired and ready replicas and a Service
// of serviceType whose slice reports readyEndpoints of total (no slice
// when total is negative). The
// Ingress that routes to it exists only when ingress is true.
func newImpactRig(
	t *testing.T, serviceType string, ingress bool,
	desired, ready, total, readyEndpoints float64,
) *impactRig {
	r := &impactRig{rig: newRig(t, Config{})}
	r.pod, r.deploy = workload(r.rig, "web")
	r.service = entity(kube.KindService, "web")
	r.slice = entity(kube.KindEndpointSlice, "web-x")
	r.observeWith(r.deploy, map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(desired),
		kube.AttrReadyReplicas:  inventory.Number(ready),
		kube.AttrTemplateLabels: inventory.Text("app=web"),
	})
	r.observeWith(r.service, map[string]inventory.Value{
		kube.AttrSelector:    inventory.Text("app=web"),
		kube.AttrServiceType: inventory.Text(serviceType),
	})
	if total >= 0 {
		r.observeWith(r.slice, map[string]inventory.Value{
			kube.AttrEndpoints:      inventory.Number(total),
			kube.AttrEndpointsReady: inventory.Number(readyEndpoints),
		})
		r.relate(r.slice, inventory.RoutesTo, r.pod)
		r.relate(r.slice, inventory.Backs, r.service)
	}
	if ingress {
		r.relate(entity(kube.KindIngress, "web"), inventory.RoutesTo,
			r.service)
	}
	return r
}

func (r *impactRig) observeWith(
	id inventory.EntityID, attrs map[string]inventory.Value,
) {
	r.t.Helper()
	if _, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: t0, Entity: id,
		Attributes: attrs,
	}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *impactRig) crash() Incident {
	r.raise(at(0), sig(r.pod, reasons.CrashLoopBackOff, detection.Critical))
	return r.only()
}

func TestPageNeedsUserImpact(t *testing.T) {
	cases := []struct {
		name        string
		serviceType string
		ingress     bool
		desired     float64
		ready       float64
		total       float64
		endpoints   float64
		want        Tier
		rule        string
	}{
		{"LoadBalancer with no ready endpoint", "LoadBalancer", false,
			2, 0, 2, 0, Page, "exposed-service-lost"},
		{"NodePort with no ready endpoint", "NodePort", false,
			2, 0, 2, 0, Page, "exposed-service-lost"},
		{"Ingress route with no ready endpoint", "ClusterIP", true,
			2, 0, 2, 0, Page, "traffic-lost"},
		{"last replica of a user-facing workload", "LoadBalancer", false,
			2, 0, -1, 0, Page, "last-replica-down"},
		{"one replica of two still serves", "LoadBalancer", false,
			2, 1, 2, 1, Notify, ""},
		{"internal Service, nothing ready", "ClusterIP", false,
			2, 0, 2, 0, Notify, ""},
		{"internal workload, last replica down", "ClusterIP", false,
			1, 0, -1, 0, Notify, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newImpactRig(t, tc.serviceType, tc.ingress, tc.desired,
				tc.ready, tc.total, tc.endpoints)
			got := r.crash()
			if got.Tier != tc.want {
				t.Fatalf("tier = %v, want %v", got.Tier, tc.want)
			}
			if rule := pageRuleOf(&got); rule != tc.rule {
				t.Fatalf("page rule = %q, want %q", rule, tc.rule)
			}
		})
	}
}

// Without a critical member nothing pages, whatever is lost: a warning
// about a user-facing workload is a notification.
func TestPageNeedsACriticalMemberToo(t *testing.T) {
	r := newImpactRig(t, "LoadBalancer", false, 2, 0, 2, 0)
	r.raise(at(0), sig(r.pod, reasons.CrashLoopBackOff, detection.Warning))
	if got := r.only().Tier; got != Notify {
		t.Fatalf("tier = %v, want notify", got)
	}
}
