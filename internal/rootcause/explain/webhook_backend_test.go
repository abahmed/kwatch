package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// webhooksBehind adds fail-closed validating webhooks that call service,
// each failing with mode, and returns the first of them.
func webhooksBehind(
	f *fixture, service inventory.EntityID, mode detection.Mode,
	names ...string,
) inventory.EntityID {
	var first inventory.EntityID
	for i, name := range names {
		hook := inventory.CoreID(kube.KindValidatingHook, "", name)
		f.add(hook)
		f.relate(hook, inventory.Serves, service)
		f.fail(hook, mode, failingH, 1, "")
		if i == 0 {
			first = hook
		}
	}
	return first
}

// Three webhooks behind one Service with no ready endpoints are one
// incident rooted at the Service, not three.
func TestExplainWebhooksShareBackendService(t *testing.T) {
	f := newFixture(t)
	service := inventory.CoreID(kube.KindService, "istio-system", "istiod")
	f.add(service)
	webhooksBehind(f, service, "Webhook.NoEndpoints",
		"istio-validator", "istiod-default-validator",
		"istio-sidecar-injector")

	ex := f.explain()

	if len(ex.Areas) != 1 || len(ex.Areas[0].Causes) != 1 {
		t.Fatalf("want one cause, got %+v", ex.Areas)
	}
	c := ex.Areas[0].Causes[0]
	if c.Root != service || c.Row != "service-no-endpoints" {
		t.Fatalf("root = %s row = %s, want the Service", c.Root, c.Row)
	}
	if len(c.Covers) != 3 {
		t.Fatalf("covers %d, want the three webhooks", len(c.Covers))
	}
}

// Two webhooks calling a Service that does not exist share that missing
// Service as their root.
func TestExplainWebhooksShareMissingBackend(t *testing.T) {
	f := newFixture(t)
	service := inventory.CoreID(kube.KindService, "kyverno", "kyverno-svc")
	webhooksBehind(f, service, "Webhook.BackendMissing",
		"kyverno-cleanup", "kyverno-exception")

	ex := f.explain()

	if len(ex.Areas) != 1 || len(ex.Areas[0].Causes) != 1 {
		t.Fatalf("want one cause, got %+v", ex.Areas)
	}
	c := ex.Areas[0].Causes[0]
	if c.Root != service || c.Row != "webhook-backend-missing" {
		t.Fatalf("root = %s row = %s, want the missing Service", c.Root,
			c.Row)
	}
}

// One webhook behind an empty Service is its own root: there is nothing
// to share, and the Service may be scaled to zero on purpose.
func TestExplainSingleWebhookStaysItsOwnRoot(t *testing.T) {
	f := newFixture(t)
	service := inventory.CoreID(kube.KindService, "policy", "hook")
	f.add(service)
	hook := webhooksBehind(f, service, "Webhook.NoEndpoints", "policy")

	ex := f.explain()

	if len(ex.Areas) != 1 || len(ex.Areas[0].Causes) != 1 {
		t.Fatalf("want one cause, got %+v", ex.Areas)
	}
	if got := ex.Areas[0].Causes[0].Root; got != hook {
		t.Fatalf("root = %s, want the webhook itself", got)
	}
}
