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

func ingressWaiting(
	t *testing.T, addressed bool, attrs map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	t.Helper()
	m := newTestModel()
	id := newID(kube.KindIngress, "ns", "web")
	attrs[kube.AttrLoadBalancer] = inventory.Bool(addressed)
	put(m, id, t0, attrs)
	return m, id
}

func ingressFindings(
	m *inventory.Model, id inventory.EntityID, at time.Time,
) []detection.Finding {
	return evaluate(Ingress{}, m, at, id, nil).Findings
}

func TestIngressWithoutAddressAndNoEvidenceIsSilent(t *testing.T) {
	m, id := ingressWaiting(t, false, map[string]inventory.Value{})

	assert.Empty(t, ingressFindings(m, id, t0.Add(time.Hour)))
}

func TestIngressControllerWarningReportsAfterGrace(t *testing.T) {
	m, id := ingressWaiting(t, false, map[string]inventory.Value{})
	warn(m, id, t0.Add(time.Minute), "SyncFailed", "bad backend")

	assert.Empty(t, ingressFindings(m, id, t0.Add(time.Minute)))

	got := ingressFindings(m, id, t0.Add(DefaultLoadBalancerFailing+
		time.Minute))
	require.Len(t, got, 1)
	assert.Equal(t, reasons.LoadBalancerPending, got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Contains(t, got[0].Summary, "for an address")
	assert.Contains(t, got[0].Summary, "bad backend")
}

func TestIngressBesideAddressedPeersOfItsClassIsReported(t *testing.T) {
	class := map[string]inventory.Value{
		kube.AttrIngressClass: inventory.Text("nginx"),
	}
	m, id := ingressWaiting(t, false, class)
	put(m, newID(kube.KindIngressClass, "", "nginx"), t0, nil)
	peer := newID(kube.KindIngress, "ns", "peer")
	put(m, peer, t0, map[string]inventory.Value{
		kube.AttrIngressClass: inventory.Text("nginx"),
		kube.AttrLoadBalancer: inventory.Bool(true),
	})

	got := ingressFindings(m, id, t0.Add(DefaultLoadBalancerPending))
	require.Len(t, got, 1)
	assert.Equal(t, reasons.LoadBalancerPending, got[0].Reason)
	assert.Contains(t, got[0].Summary, "ingress class nginx")
}

func TestIngressPeersOfAnotherClassAreNoEvidence(t *testing.T) {
	m, id := ingressWaiting(t, false, map[string]inventory.Value{
		kube.AttrIngressClass: inventory.Text("nginx"),
	})
	put(m, newID(kube.KindIngressClass, "", "nginx"), t0, nil)
	put(m, newID(kube.KindIngress, "ns", "peer"), t0,
		map[string]inventory.Value{
			kube.AttrIngressClass: inventory.Text("other"),
			kube.AttrLoadBalancer: inventory.Bool(true),
		})

	assert.Empty(t, ingressFindings(m, id, t0.Add(time.Hour)))
}

func TestIngressWithAddressIsNotReported(t *testing.T) {
	m, id := ingressWaiting(t, true, map[string]inventory.Value{})

	assert.Empty(t, ingressFindings(m, id, t0.Add(time.Hour)))
}

func TestIngressLosingItsAddressIsAWarning(t *testing.T) {
	m, id := ingressWaiting(t, true, map[string]inventory.Value{})
	put(m, id, t0.Add(time.Minute), map[string]inventory.Value{
		kube.AttrLoadBalancer: inventory.Bool(false),
	})

	got := ingressFindings(m, id, t0.Add(10*time.Minute))
	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

func TestIngressWithMissingClassLeavesItToIngressClassMissing(t *testing.T) {
	m, id := ingressWaiting(t, false, map[string]inventory.Value{
		kube.AttrIngressClass: inventory.Text("gone"),
	})

	got := ingressFindings(m, id, t0.Add(time.Hour))
	require.Len(t, got, 1)
	assert.Equal(t, reasons.IngressClassMissing, got[0].Reason)
}
