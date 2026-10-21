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

// emptyService builds a Service in shop selecting selector, backed by
// an EndpointSlice that lists nothing, and an Ingress sending traffic
// to it when routed is true. A healthy Deployment of pods labelled
// app=api runs in the namespace.
func emptyService(
	selector string, routed bool,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	svc := newID(kube.KindService, "shop", "api")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrServiceType: inventory.Text("ClusterIP"),
		kube.AttrSelector:    inventory.Text(selector),
	})
	slice := newID(kube.KindEndpointSlice, "shop", "api-1")
	put(m, slice, t0, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(0),
		kube.AttrEndpointsReady: inventory.Number(0),
	})
	link(m, slice, inventory.Backs, svc)
	put(m, newID(kube.KindDeployment, "shop", "api"), t0,
		map[string]inventory.Value{
			kube.AttrTemplateLabels: inventory.Text("app=api"),
			kube.AttrReplicas:       inventory.Number(2),
		})
	put(m, newID(kube.KindPod, "shop", "api-1"), t0,
		map[string]inventory.Value{
			kube.AttrLabels: inventory.Text("app=api"),
			kube.AttrPhase:  inventory.Text("Running"),
			kube.AttrReady:  inventory.Bool(true),
		})
	if routed {
		ing := newID(kube.KindIngress, "shop", "web")
		put(m, ing, t0, nil)
		link(m, ing, inventory.RoutesTo, svc)
	}
	return m, svc
}

func TestServiceSelectingNothingBehindIngressIsCritical(t *testing.T) {
	m, id := emptyService("app=apii", true)

	early := evaluate(Service{}, m, t0.Add(30*time.Second), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, 30*time.Second, early.RecheckAfter)

	got := evaluate(Service{}, m, t0.Add(DefaultNoEndpoints), id,
		nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceNoEndpoints, got[0].Reason)
	assert.Equal(t, detection.Critical, got[0].Severity)
	assert.False(t, got[0].Symptom)
	assert.Equal(t, "Service api selects app=apii; no pod or workload "+
		"template in shop has those labels", got[0].Summary)
}

func TestServiceSelectingNothingBehindLoadBalancerIsCritical(t *testing.T) {
	for _, serviceType := range []string{"LoadBalancer", "NodePort"} {
		m, id := emptyService("app=apii", false)
		put(m, id, t0, map[string]inventory.Value{
			kube.AttrServiceType: inventory.Text(serviceType),
			kube.AttrSelector:    inventory.Text("app=apii"),
		})

		got := evaluate(Service{}, m, t0.Add(DefaultNoEndpoints), id,
			nil).Findings

		require.Len(t, got, 1, serviceType)
		assert.Equal(t, reasons.ServiceNoEndpoints, got[0].Reason)
	}
}

// A workload scaled to zero has no pod but still carries the labels:
// the Service is its backend when it runs again.
func TestServiceOfScaledToZeroWorkloadStaysQuiet(t *testing.T) {
	m, id := emptyService("app=api", true)
	put(m, newID(kube.KindPod, "shop", "api-1"), t0, map[string]inventory.Value{
		kube.AttrLabels: inventory.Text("app=other"),
	})

	got := evaluate(Service{}, m, t0.Add(time.Hour), id, nil).Findings

	assert.Empty(t, got)
}

func TestServiceSelectingNothingWithoutExposureStaysQuiet(t *testing.T) {
	m, id := emptyService("app=apii", false)

	got := evaluate(Service{}, m, t0.Add(time.Hour), id, nil).Findings

	assert.Empty(t, got)
}

func TestServiceSelectingNothingWaitsForEverySourceToSync(t *testing.T) {
	for _, kind := range []inventory.Kind{kube.KindPod,
		kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
		kube.KindReplicaSet, kube.KindJob} {
		m, id := emptyService("app=apii", true)
		synced := func(k inventory.Kind) bool { return k != kind }

		got := evaluate(Service{}, m, t0.Add(time.Hour), id, synced)

		assert.Empty(t, got.Findings, kind)
		assert.Contains(t, got.Unsynced, kind)
	}
}

// Pods that exist but are all Pending have no address, so the slice is
// empty although the Service is not scaled to zero.
func TestServiceWithOnlyPendingPodsHasNoReadyBackends(t *testing.T) {
	m, id := emptyService("app=api", false)
	for _, name := range []string{"api-1", "api-2"} {
		put(m, newID(kube.KindPod, "shop", name), t0,
			map[string]inventory.Value{
				kube.AttrLabels:  inventory.Text("app=api"),
				kube.AttrPhase:   inventory.Text("Pending"),
				kube.AttrCreated: inventory.Time(t0.Add(-time.Hour)),
			})
	}
	now := t0.Add(time.Hour)
	evaluate(Service{}, m, now, id, nil)

	got := evaluate(Service{}, m, now.Add(DefaultNoEndpoints), id,
		nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceNoEndpoints, got[0].Reason)
	assert.True(t, got[0].Symptom)
	assert.Contains(t, got[0].Summary, "no ready backends")
}

func TestServiceWithRunningPodsAndEmptySliceStaysQuiet(t *testing.T) {
	m, id := emptyService("app=api", false)

	got := evaluate(Service{}, m, t0.Add(time.Hour), id, nil).Findings

	assert.Empty(t, got)
}

// A pod shutting down on purpose is neither a backend nor an unready
// one: three endpoints of which one terminates is two of two.
func TestServiceIgnoresTerminatingEndpoints(t *testing.T) {
	m, id := serviceWithSlice(3, 2, "80", "80")
	put(m, newID(kube.KindEndpointSlice, "default", "api-1"), t0,
		map[string]inventory.Value{
			kube.AttrEndpoints:            inventory.Number(3),
			kube.AttrEndpointsReady:       inventory.Number(2),
			kube.AttrEndpointsTerminating: inventory.Number(1),
		})

	got := evaluate(Service{}, m, t0.Add(time.Hour), id, nil).Findings

	assert.Empty(t, got)
}

func TestServiceCountsUnreadyEndpointsThatAreNotTerminating(t *testing.T) {
	m, id := serviceWithSlice(3, 2, "80", "80")

	got := evaluate(Service{}, m, t0.Add(DefaultDegradedBackends), id,
		nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceBackendsDegraded, got[0].Reason)
}
