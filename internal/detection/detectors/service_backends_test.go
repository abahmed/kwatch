package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func serviceWithSlice(total, ready float64, slicePorts, target string,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	svc := newID(kube.KindService, "default", "api")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrServiceType: inventory.Text("ClusterIP"),
		kube.AttrSelector:    inventory.Text("app=api"),
		kube.AttrTargetPorts: inventory.Text(target),
	})
	slice := newID(kube.KindEndpointSlice, "default", "api-1")
	put(m, slice, t0, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(total),
		kube.AttrEndpointsReady: inventory.Number(ready),
		kube.AttrEndpointPorts:  inventory.Text(slicePorts),
	})
	link(m, slice, inventory.Backs, svc)
	return m, svc
}

func TestServiceNoReadyBackends(t *testing.T) {
	m, id := serviceWithSlice(2, 0, "80", "80")
	early := evaluate(Service{}, m, t0.Add(30*time.Second), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, 30*time.Second, early.RecheckAfter)

	got := evaluate(Service{}, m, t0.Add(time.Minute), id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceNoEndpoints, got[0].Reason)
	assert.Equal(t, detection.Critical, got[0].Severity)
	assert.True(t, got[0].Symptom)
	assert.NotEmpty(t, got[0].Summary)
}

func TestServiceDegradedBackends(t *testing.T) {
	m, id := serviceWithSlice(4, 3, "80", "80")
	early := evaluate(Service{}, m, t0.Add(4*time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, time.Minute, early.RecheckAfter)

	got := evaluate(Service{}, m, t0.Add(5*time.Minute), id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceBackendsDegraded, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, "3 of 4 Service backends are ready", got[0].Summary)
}

func TestServiceBackendsQuiet(t *testing.T) {
	now := t0.Add(time.Hour)
	m, id := serviceWithSlice(0, 0, "", "80")
	assert.Empty(t, evaluate(Service{}, m, now, id, nil).Findings,
		"scaled to zero on purpose")

	m, id = serviceWithSlice(2, 2, "80", "80")
	assert.Empty(t, evaluate(Service{}, m, now, id, nil).Findings)

	m = newTestModel()
	bare := newID(kube.KindService, "default", "bare")
	put(m, bare, t0, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=x"),
	})
	assert.Empty(t, evaluate(Service{}, m, now, bare, nil).Findings,
		"no slices known")
	assert.Equal(t, "service", Service{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindService}, Service{}.Kinds())
}

func TestServicePortMismatch(t *testing.T) {
	m, id := serviceWithSlice(2, 2, "8080", "80,http")
	early := evaluate(Service{}, m, t0.Add(30*time.Second), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, 30*time.Second, early.RecheckAfter)

	got := evaluate(Service{}, m, t0.Add(time.Minute), id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServicePortMismatch, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "80")
}

func TestServicePortMatchQuiet(t *testing.T) {
	m, id := serviceWithSlice(2, 2, "8080", "8080,http")
	assert.Empty(t, evaluate(Service{}, m, t0.Add(time.Hour), id,
		nil).Findings)
}

func TestIngressMetadataAndSyncGate(t *testing.T) {
	m := newTestModel()
	ing := newID(kube.KindIngress, "default", "web")
	put(m, ing, t0, nil)
	link(m, ing, inventory.RoutesTo, newID(kube.KindService, "default", "x"))
	unsynced := func(k inventory.Kind) bool { return k != kube.KindService }
	assert.Empty(t, evaluate(Ingress{}, m, t0, ing, unsynced).Findings)
	assert.Len(t, evaluate(Ingress{}, m, t0, ing, nil).Findings, 1)
	assert.Equal(t, "ingress", Ingress{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindIngress}, Ingress{}.Kinds())
	assert.Equal(t, "egress-policy", EgressPolicy{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindNetworkPolicy},
		EgressPolicy{}.Kinds())
}

func TestMissingReferenceKinds(t *testing.T) {
	assert.Equal(t, reasons.ServiceAccountMissing,
		missingReason(kube.KindAccount))
	assert.Empty(t, missingReason(kube.KindPVC))
	assert.Equal(t, "missing-reference", Missing{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindPod}, Missing{}.Kinds())
	assert.Equal(t, "certificate", Certificate{}.Name())
	assert.Equal(t, []inventory.Kind{kube.KindSecret}, Certificate{}.Kinds())
}
