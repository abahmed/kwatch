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

func customEntity(m *inventory.Model, kind inventory.Kind, ns, name string,
	attrs map[string]inventory.Value,
) inventory.EntityID {
	id := newID(kind, ns, name)
	if attrs == nil {
		attrs = map[string]inventory.Value{}
	}
	attrs[kube.AttrCustom] = inventory.Bool(true)
	put(m, id, t0, attrs)
	return id
}

func TestCustomReportsCRDNotEstablished(t *testing.T) {
	m := newTestModel()
	id := customEntity(m, "customresourcedefinition", "",
		"widgets.example.com", nil)
	setCondition(m, id, "NamesAccepted", "False", "AllNamesConflict",
		"plural widgets is already in use", t0)

	eval := evaluate(Custom{}, m, t0.Add(3*time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "CRD.NotEstablished", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "AllNamesConflict")

	setCondition(m, id, "NamesAccepted", "True", "NoConflicts", "", t0)
	setCondition(m, id, "Established", "True", "InitialNamesAccepted",
		"", t0)
	assert.Empty(t, evaluate(Custom{}, m, t0.Add(time.Hour), id,
		nil).Findings)
}

func csr(m *inventory.Model, signer string, issued bool,
) inventory.EntityID {
	return customEntity(m, "certificatesigningrequest", "", "csr-1",
		map[string]inventory.Value{
			kube.AttrSignerName:        inventory.Text(signer),
			kube.AttrCertificateIssued: inventory.Bool(issued),
		})
}

func TestCustomReportsDeniedCertificateRequest(t *testing.T) {
	m := newTestModel()
	id := csr(m, "example.com/signer", false)
	setCondition(m, id, "Denied", "True", "PolicyDenied", "", t0)

	eval := evaluate(Custom{}, m, t0, id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.CertificateDenied, eval.Findings[0].Reason)
	assert.Equal(t, "Certificate.Denied", string(eval.Findings[0].Mode))
}

func TestCustomReportsApprovedCertificateNotIssued(t *testing.T) {
	m := newTestModel()
	id := csr(m, "example.com/signer", false)
	setCondition(m, id, "Approved", "True", "AutoApproved", "", t0)

	early := evaluate(Custom{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)

	eval := evaluate(Custom{}, m, t0.Add(6*time.Minute), id, nil)
	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Certificate.NotIssued", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "example.com/signer")
}

func TestCustomReportsPendingKubeletServingRequest(t *testing.T) {
	m := newTestModel()
	serving := csr(m, "kubernetes.io/kubelet-serving", false)

	eval := evaluate(Custom{}, m, t0.Add(16*time.Minute), serving, nil)
	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Certificate.NotIssued", string(eval.Findings[0].Mode))

	m2 := newTestModel()
	other := csr(m2, "kubernetes.io/kube-apiserver-client", false)
	assert.Empty(t, evaluate(Custom{}, m2, t0.Add(time.Hour), other,
		nil).Findings, "only serving requests need an approver")

	m3 := newTestModel()
	done := csr(m3, "kubernetes.io/kubelet-serving", true)
	assert.Empty(t, evaluate(Custom{}, m3, t0.Add(time.Hour), done,
		nil).Findings)
}

func TestCustomReportsUnallocatedClaimOfPendingPod(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "train-0")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
	})
	claim := customEntity(m, "resourceclaim", "ns", "train-0-gpu",
		map[string]inventory.Value{kube.AttrAllocated: inventory.Bool(false)})
	link(m, claim, inventory.OwnedBy, pod)

	early := evaluate(Custom{}, m, t0.Add(time.Minute), claim, nil)
	assert.Empty(t, early.Findings)

	eval := evaluate(Custom{}, m, t0.Add(6*time.Minute), claim, nil)
	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Device.Unallocated", string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "train-0")

	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running"),
	})
	assert.Empty(t, evaluate(Custom{}, m, t0.Add(time.Hour), claim,
		nil).Findings)
}

func TestCustomIgnoresStandaloneUnallocatedClaim(t *testing.T) {
	m := newTestModel()
	claim := customEntity(m, "resourceclaim", "ns", "shared",
		map[string]inventory.Value{kube.AttrAllocated: inventory.Bool(false)})

	assert.Empty(t, evaluate(Custom{}, m, t0.Add(time.Hour), claim,
		nil).Findings)
}

func TestCustomReportsEachFailingListener(t *testing.T) {
	m := newTestModel()
	gw := customEntity(m, "gateway", "ns", "public",
		map[string]inventory.Value{
			kube.AttrListenerProblems: inventory.Text(
				"listener a: OverlappingTLSConfig=True; " +
					"listener b: Programmed=False (InvalidCertificateRef)"),
		})

	eval := evaluate(Custom{}, m, t0.Add(3*time.Minute), gw, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, reasons.CustomResourceFailure, f.Reason)
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: "listener", Value: "listener a: OverlappingTLSConfig=True"})
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: "listener",
		Value: "listener b: Programmed=False (InvalidCertificateRef)"})
}

// TestCustomReportsRouteBackendMissing: a Gateway API route that sends
// traffic to a Service that does not exist is reported at once, as an
// Ingress is, without waiting for the condition's grace period.
func TestCustomReportsRouteBackendMissing(t *testing.T) {
	for _, exists := range []bool{false, true} {
		m := newTestModel()
		route := customEntity(m, "httproute", "shop", "web", nil)
		service := newID(kube.KindService, "shop", "checkout-v2")
		if exists {
			put(m, service, t0, nil)
		}
		link(m, route, inventory.RoutesTo, service)

		eval := evaluate(Custom{}, m, t0.Add(DefaultBackendGrace), route,
			nil)

		if exists {
			assert.Empty(t, eval.Findings)
			continue
		}
		require.Len(t, eval.Findings, 1)
		assert.Equal(t, reasons.IngressBackendNotFound,
			eval.Findings[0].Reason)
		assert.Equal(t, detection.Critical, eval.Findings[0].Severity)
		assert.Contains(t, eval.Findings[0].Summary, "checkout-v2")
	}
}

// TestRouteBackendsWaitForServices: before the Services are listed a
// missing one proves nothing.
func TestRouteBackendsWaitForServices(t *testing.T) {
	m := newTestModel()
	route := customEntity(m, "grpcroute", "shop", "rpc", nil)
	link(m, route, inventory.RoutesTo,
		newID(kube.KindService, "shop", "pricing"))
	notServices := func(kind inventory.Kind) bool {
		return kind != kube.KindService
	}
	eval := evaluate(Custom{}, m, t0.Add(time.Second), route, notServices)
	assert.Empty(t, eval.Findings)
}

// TestRouteBackendsIgnoreOtherKinds: a route may send traffic to a
// ServiceImport or a vendor backend; only a Service target is judged
// against the Services.
func TestRouteBackendsIgnoreOtherKinds(t *testing.T) {
	m := newTestModel()
	route := customEntity(m, "httproute", "shop", "web", nil)
	link(m, route, inventory.RoutesTo,
		newID("serviceimport", "shop", "remote"))

	eval := evaluate(Custom{}, m, t0.Add(time.Second), route, nil)

	assert.Empty(t, eval.Findings)
}
