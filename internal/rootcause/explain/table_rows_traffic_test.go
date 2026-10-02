package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var trafficRowCases = []rowCase{
	{row: "routed-missing", want: "service/shop/web-v2",
		build: func(f *fixture) inventory.EntityID {
			ingress := inventory.CoreID(kube.KindIngress, "shop", "web")
			f.add(ingress)
			f.relate(ingress, inventory.RoutesTo,
				inventory.CoreID(kube.KindService, "shop", "web-v2"))
			f.fail(ingress, "BackendMissing", failingH, 2, "")
			return ingress
		}},
	{row: "certificate-expired", want: "secret/shop/api-tls",
		build: func(f *fixture) inventory.EntityID {
			return certCase(f, "Cert.Expired",
				"tls: failed to verify certificate: x509: certificate "+
					"has expired or is not yet valid")
		}},
}

// certCase gives the Secret api-tls a certificate finding in mode and
// crashes the pods that use it with text.
func certCase(
	f *fixture, mode detection.Mode, text string,
) inventory.EntityID {
	secret := inventory.CoreID(kube.KindSecret, "shop", "api-tls")
	f.add(secret)
	f.fail(secret, mode, failingH, 1, "")
	pods := f.workload("shop", "api", 2)
	for _, pod := range pods {
		f.relate(pod, inventory.References, secret)
		f.fail(containerOf(pod), "CrashLoop", failingH, 2, text)
	}
	return containerOf(pods[0])
}

// TestCertificateNeedsTLSErrors: an expired certificate next to a crash
// that says nothing about TLS is not blamed for it.
func TestCertificateNeedsTLSErrors(t *testing.T) {
	f := newFixture(t)
	effect := certCase(f, "Cert.Expired", "panic: nil map")
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Root.Kind == kube.KindSecret {
		t.Fatalf("an unrelated crash blamed %s", c.Root)
	}
}

// TestRouteFailingOnMissingBackend: a Gateway API route reports a
// missing backend only through its failed condition, which the generic
// finding names NotReconciling. The Service it routes to, which does
// not exist, is still the cause, even though the route's own edit
// named it.
func TestRouteFailingOnMissingBackend(t *testing.T) {
	f := newFixture(t)
	route := inventory.NewEntityID("gateway.networking.k8s.io", "httproute",
		"shop", "web")
	f.add(route)
	f.change(route, 1, "spec.rules[0].backendRefs[0].name")
	f.relate(route, inventory.RoutesTo,
		inventory.CoreID(kube.KindService, "shop", "checkout-v2"))
	f.fail(route, "NotReconciling", degradedH, 2,
		"Reports ResolvedRefs=False (BackendNotFound)")
	c := requireCause(t, f.explain(), route, "service/shop/checkout-v2")
	if c.Row != "routed-missing" {
		t.Fatalf("row = %s, want routed-missing", c.Row)
	}
}
