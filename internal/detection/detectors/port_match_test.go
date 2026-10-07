package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// portModel is a Service api (selector app=api, specs) and one pod that
// declares podPorts.
func portModel(specs, podPorts string, extra map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	svc := newID(kube.KindService, "shop", "api")
	attrs := map[string]inventory.Value{
		kube.AttrSelector:         inventory.Text("app=api"),
		kube.AttrServicePortSpecs: inventory.Text(specs),
	}
	for name, value := range extra {
		attrs[name] = value
	}
	put(m, svc, t0, attrs)
	pod := newID(kube.KindPod, "shop", "api-1")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrLabels:   inventory.Text("app=api"),
		kube.AttrPodPorts: inventory.Text(podPorts),
	})
	return m, svc
}

func TestServiceNamedTargetPortMissingIsDefinitive(t *testing.T) {
	m, id := portModel("80:web:web", "http=80", nil)
	now := t0.Add(2 * time.Minute)
	got := evaluate(Service{}, m, now, id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServicePortMismatch, got[0].Reason)
	assert.Contains(t, got[0].Summary, "port named web")
	assert.Contains(t, got[0].Summary, "80 (http)")
}

func TestServiceNamedTargetPortWaitsForGrace(t *testing.T) {
	m, id := portModel("80:web:web", "http=80", nil)
	early := evaluate(Service{}, m, t0.Add(10*time.Second), id, nil)
	assert.Empty(t, early.Findings)
}

func TestServiceNumberTargetNeedsRefusedProbe(t *testing.T) {
	now := t0.Add(2 * time.Minute)
	m, id := portModel("80::8080", "http=80", nil)
	assert.Empty(t, evaluate(Service{}, m, now, id, nil).Findings,
		"declared ports are informational without present-tense proof")

	m, id = portModel("80::8080", "http=80", map[string]inventory.Value{
		kube.AttrHealthy:          inventory.Bool(false),
		kube.AttrProbeFailureKind: inventory.Text(kube.FailureRefused),
	})
	got := evaluate(Service{}, m, now, id, nil).Findings
	var found bool
	for _, f := range got {
		if f.Reason == reasons.ServicePortMismatch {
			found = true
			assert.Contains(t, f.Summary,
				"sends traffic to port 8080, but its pods listen on 80 (http)")
			assert.Contains(t, f.Summary, "refused")
		}
	}
	assert.True(t, found)
}

func TestServicePortsThatMatchAreQuiet(t *testing.T) {
	now := t0.Add(2 * time.Minute)
	for _, specs := range []string{"80::80", "80::http", "443:tls:8443"} {
		m, id := portModel(specs, "http=80,8443", map[string]inventory.Value{
			kube.AttrHealthy:          inventory.Bool(false),
			kube.AttrProbeFailureKind: inventory.Text(kube.FailureRefused),
		})
		for _, f := range evaluate(Service{}, m, now, id, nil).Findings {
			assert.NotEqual(t, reasons.ServicePortMismatch, f.Reason, specs)
		}
	}
}

func ingressPortModel(backend string) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	svc := newID(kube.KindService, "shop", "api")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrServicePortSpecs: inventory.Text("80:http:8080"),
	})
	ing := newID(kube.KindIngress, "shop", "shop")
	put(m, ing, t0, map[string]inventory.Value{
		kube.AttrIngressBackendPorts: inventory.Text("/api\tapi\t" + backend),
	})
	link(m, ing, inventory.RoutesTo, svc)
	return m, ing
}

func TestIngressBackendPortMissing(t *testing.T) {
	m, ing := ingressPortModel("9090")
	now := t0.Add(DefaultBackendGrace)
	got := evaluate(Ingress{}, m, now, ing, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.IngressBackendPortMissing, got[0].Reason)
	assert.Equal(t, "Ingress routes /api to Service api port 9090, "+
		"which api doesn't have (it has 80/http)", got[0].Summary)
	assert.Empty(t, evaluate(Ingress{}, m, t0.Add(time.Second), ing,
		nil).Findings, "grace for manifests applied in any order")
}

func TestIngressBackendPortPresentIsQuiet(t *testing.T) {
	now := t0.Add(DefaultBackendGrace)
	for _, port := range []string{"80", "http"} {
		m, ing := ingressPortModel(port)
		assert.Empty(t, evaluate(Ingress{}, m, now, ing, nil).Findings, port)
	}
}
