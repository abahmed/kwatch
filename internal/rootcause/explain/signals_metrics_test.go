package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestMetricsAPIBlamedBeforeItsOwnStatusSays(t *testing.T) {
	f := newFixture(t)
	api := inventory.CoreID(kube.KindAPIService, "",
		"v1beta1.metrics.k8s.io")
	f.add(api)
	var first inventory.EntityID
	for _, ns := range []string{"shop", "billing"} {
		hpa := inventory.CoreID(kube.KindHPA, ns, "web")
		f.add(hpa)
		f.fail(hpa, "Scaling.NoMetrics", degradedH, 2,
			"unable to fetch metrics from resource metrics API: the "+
				"server is currently unable to handle the request "+
				"(get pods.metrics.k8s.io)")
		if first.IsZero() {
			first = hpa
		}
	}
	requireCause(t, f.explain(), first, api.String())
}

func TestMetricsAPIHealthyWithoutTheSignal(t *testing.T) {
	f := newFixture(t)
	api := inventory.CoreID(kube.KindAPIService, "",
		"v1beta1.metrics.k8s.io")
	hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
	f.add(api, hpa)
	f.fail(hpa, "Scaling.NoMetrics", degradedH, 2, "max replicas reached")
	if c, ok := f.explain().CauseOf(hpa); ok && c.Root == api {
		t.Fatalf("a healthy metrics API was blamed: %+v", c)
	}
}

func TestClassifyPullStatusWithoutCode(t *testing.T) {
	cut := `failed to resolve reference: pulling from host ` +
		`registry.corp.example failed with status code [manifests v1]: …`
	if got := classifyPull(cut); got != pullStatus {
		t.Fatalf("class = %q, want %q", got, pullStatus)
	}
	missing := `pulling from host r.example failed with status code ` +
		`[manifests v1]: 404 not found`
	if got := classifyPull(missing); got != pullImage {
		t.Fatalf("a missing image blamed the registry: %q", got)
	}
}
