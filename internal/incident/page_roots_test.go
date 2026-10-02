package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// observe adds id to model with text attributes.
func observe(t *testing.T, model *inventory.Model, id inventory.EntityID,
	attrs map[string]string) {
	t.Helper()
	values := map[string]inventory.Value{}
	for name, value := range attrs {
		values[name] = inventory.Text(value)
	}
	if _, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: t0, Entity: id,
		Attributes: values,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAdmissionBlockedNeedsFailClosedWebhook: a webhook blamed for
// failed creates pages only when it fails closed. Its own finding is
// not needed: its endpoints may be ready while its calls time out.
func TestAdmissionBlockedNeedsFailClosedWebhook(t *testing.T) {
	hook := inventory.CoreID(kube.KindValidatingHook, "", "policy")
	rs := entity(kube.KindReplicaSet, "web-1")
	cases := map[string]struct {
		policy string
		rule   string
		want   Tier
	}{
		"fail closed pages":     {"Fail", webhookRejectsRule, Page},
		"ignoring notifies":     {"Ignore", webhookRejectsRule, Notify},
		"another rule notifies": {"Fail", "owner-failing", Notify},
		"one fail-closed hook pages": {"Ignore,Fail", webhookRejectsRule,
			Page},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			observe(t, model, hook,
				map[string]string{kube.AttrFailurePolicy: c.policy})
			p := incidentOf(hook, nil,
				sig(rs, "FailedCreate", detection.Warning))
			p.Cause = &rootcause.CauseRecord{Root: hook, Rule: c.rule}
			p.admissionBlocked = admissionBlocked(model, p)
			if got := tier(p); got != c.want {
				t.Fatalf("tier = %v, want %v", got, c.want)
			}
		})
	}
}

// TestRoutedMissingServicePages: a Service an Ingress or route sends
// traffic to that does not exist loses every request, so it pages even
// when the route reports it only as a warning.
func TestRoutedMissingServicePages(t *testing.T) {
	route := inventory.NewEntityID("gateway.networking.k8s.io",
		"httproute", "shop", "web")
	cases := map[string]struct {
		exists bool
		want   Tier
	}{
		"missing Service pages":     {false, Page},
		"existing Service notifies": {true, Notify},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			model := inventory.NewModel(inventory.Options{})
			service := entity(kube.KindService, "checkout-v2")
			if c.exists {
				observe(t, model, service, nil)
			}
			observe(t, model, route, nil)
			if _, err := model.Apply(inventory.Observation{
				Kind: inventory.Related, Source: "test", At: t0,
				Entity: route, Relation: inventory.RoutesTo,
				Targets: []inventory.EntityID{service},
			}); err != nil {
				t.Fatal(err)
			}
			p := incidentOf(service, []inventory.EntityID{route},
				sig(route, reasons.CustomResourceFailure,
					detection.Warning))
			p.trafficLost = trafficLost(model, p)
			p.routedMissing = routedMissing(model, p)
			if got := tier(p); got != c.want {
				t.Fatalf("tier = %v, want %v", got, c.want)
			}
		})
	}
}
