package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A Deployment whose edit was never processed was not broken by it: the
// controller has not read it yet. Its own change is no rival for the
// throttled API server that keeps controllers from reconciling (the
// scenario apiserver-throttling-stalls-controllers).
func TestOwnChangeDoesNotExplainAnUnprocessedChange(t *testing.T) {
	f := newFixture(t)
	api := inventory.CoreID(kindAPIServer, "", "kube-apiserver")
	f.add(api)
	f.fail(api, detection.ModeThrottlingAPIServer, failingH, 1, "")
	cart := inventory.CoreID(kube.KindDeployment, "shop", "cart")
	f.add(cart)
	f.change(cart, 0, "spec.template.spec.containers")
	f.fail(cart, detection.ModeNotReconciling, failingH, 1, "")
	f.findings[cart][0].Reason = reasons.GenerationLagging

	c, ok := f.explain().CauseOf(cart)
	if ok && c.Row == "own-change" {
		t.Fatalf("an unprocessed change blamed for itself: %+v", c)
	}
}

// A resource whose controller read the edit and reported an error was
// broken by it: the own change stays its explanation (the scenarios
// route-not-accepted and borderline-operator-upgrade-and-edit).
func TestOwnChangeExplainsAResourceItsControllerRejected(t *testing.T) {
	f := newFixture(t)
	route := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(route)
	f.change(route, 0, "spec.parentRefs")
	f.fail(route, detection.ModeNotReconciling, failingH, 1, "")
	f.findings[route][0].Reason = reasons.CustomResourceFailure

	c, ok := f.explain().CauseOf(route)
	if !ok || c.Row != "own-change" {
		t.Fatalf("a rejected edit not blamed for itself: %+v", c)
	}
}
