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

// zeroShop builds shop/api scaled to zero, selected by Service api,
// which Ingress web routes to. The scale-down was 10 minutes ago.
func zeroShop() (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	api := newID(kube.KindDeployment, "shop", "api")
	put(m, api, t0.Add(-10*time.Minute), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
	})
	svc := newID(kube.KindService, "shop", "api")
	put(m, svc, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrSelector:    inventory.Text("app=api"),
		kube.AttrServiceType: inventory.Text("ClusterIP"),
	})
	ingress := newID(kube.KindIngress, "shop", "web")
	put(m, ingress, t0.Add(-time.Hour), nil)
	link(m, ingress, inventory.RoutesTo, svc)
	return m, api
}

func detectZero(m *inventory.Model, id inventory.EntityID,
) []detection.Finding {
	return Workload{}.Detect(testDetectorContext(m, t0), entityOf(m, id))
}

func TestWorkloadReportsScaleToZeroWhileAnIngressStillRoutesToIt(t *testing.T) {
	m, api := zeroShop()

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ScaledToZeroRouted, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, "Is scaled to 0 but Ingress shop/web still routes "+
		"traffic to it via Service api", got[0].Summary)
}

// A workload parked at zero for a long time is no news: it goes to the
// digest. The age comes from the latest spec write on the object.
func TestWorkloadScaledToZeroForDaysIsOnlyInformational(t *testing.T) {
	m, api := zeroShop()
	put(m, api, t0.Add(-10*time.Minute), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
		kube.AttrSpecWritten:    inventory.Time(t0.Add(-48 * time.Hour)),
	})

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ScaledToZeroRouted, got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
}

// A scale-down kwatch saw, long ago, counts the same.
func TestWorkloadScaledToZeroDaysAgoPerHistoryIsOnlyInformational(
	t *testing.T,
) {
	m, api := zeroShop()
	scaledAt := t0.Add(-3 * 24 * time.Hour)
	m.Apply(inventory.Observation{
		Kind: inventory.Changed, Source: "test", At: scaledAt, Entity: api,
		Change: inventory.Change{Entity: api, At: scaledAt,
			Actor: "kubectl-scale", Fields: []inventory.FieldChange{{
				Path: "spec.replicas", Before: "3", After: "0"}}},
	})

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Info, got[0].Severity)
}

// A scale-down within the last day still notifies.
func TestWorkloadScaledToZeroRecentlyStillWarns(t *testing.T) {
	m, api := zeroShop()
	put(m, api, t0.Add(-10*time.Minute), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
		kube.AttrSpecWritten:    inventory.Time(t0.Add(-23 * time.Hour)),
	})

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

func TestWorkloadNamesWhoScaledItToZeroAndWhen(t *testing.T) {
	m, api := zeroShop()
	scaledAt := t0.Add(-9 * time.Minute)
	m.Apply(inventory.Observation{
		Kind: inventory.Changed, Source: "test", At: scaledAt, Entity: api,
		Change: inventory.Change{Entity: api, At: scaledAt,
			Actor: "kubectl-scale", Fields: []inventory.FieldChange{{
				Path: "spec.replicas", Before: "3", After: "0"}}},
	})

	got := detectZero(m, api)

	require.Len(t, got, 1)
	byLabel := evidenceByLabel(got[0].Evidence)
	assert.Equal(t, "kubectl-scale", byLabel[detection.EvidenceScaledBy])
	assert.Equal(t, scaledAt.UTC().Format(time.RFC3339),
		byLabel[detection.EvidenceScaledAt])
}

func TestWorkloadReportsAScaledToZeroLoadBalancerService(t *testing.T) {
	m := newTestModel()
	api := newID(kube.KindDeployment, "shop", "api")
	put(m, api, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
	})
	put(m, newID(kube.KindService, "shop", "api"), t0,
		map[string]inventory.Value{
			kube.AttrSelector:    inventory.Text("app=api"),
			kube.AttrServiceType: inventory.Text("LoadBalancer"),
		})

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.Equal(t, "Is scaled to 0 but Service api (LoadBalancer) still "+
		"exposes it", got[0].Summary)
}

func TestWorkloadIgnoresScaleToZeroNobodyRoutesTo(t *testing.T) {
	m := newTestModel()
	api := newID(kube.KindDeployment, "shop", "api")
	put(m, api, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
	})
	put(m, newID(kube.KindService, "shop", "api"), t0,
		map[string]inventory.Value{
			kube.AttrSelector:    inventory.Text("app=api"),
			kube.AttrServiceType: inventory.Text("ClusterIP"),
		})

	assert.Empty(t, detectZero(m, api))
}

func TestWorkloadIgnoresARouteToAServiceThatSelectsOtherPods(t *testing.T) {
	m, api := zeroShop()
	put(m, newID(kube.KindService, "shop", "api"), t0,
		map[string]inventory.Value{
			kube.AttrSelector: inventory.Text("app=other")})

	assert.Empty(t, detectZero(m, api))
}

func TestWorkloadIgnoresScaleToZeroAnAutoscalerAllows(t *testing.T) {
	m, api := zeroShop()
	hpa := newID(kube.KindHPA, "shop", "api")
	put(m, hpa, t0, map[string]inventory.Value{
		kube.AttrMinReplicas: inventory.Number(0)})
	link(m, hpa, inventory.Scales, api)

	assert.Empty(t, detectZero(m, api))
}

func TestWorkloadStillReportsWhenTheAutoscalerKeepsAtLeastOne(t *testing.T) {
	m, api := zeroShop()
	hpa := newID(kube.KindHPA, "shop", "api")
	put(m, hpa, t0, map[string]inventory.Value{
		kube.AttrMinReplicas: inventory.Number(1)})
	link(m, hpa, inventory.Scales, api)

	assert.Len(t, detectZero(m, api), 1)
}

func TestWorkloadWaitsBeforeReportingAFreshScaleToZero(t *testing.T) {
	m := newTestModel()
	api := newID(kube.KindDeployment, "shop", "api")
	put(m, api, t0.Add(-time.Minute), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
	})
	svc := newID(kube.KindService, "shop", "api")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=api")})
	link(m, newID(kube.KindIngress, "shop", "web"), inventory.RoutesTo, svc)

	eval := evaluate(NewWorkload(0), m, t0, api, nil)

	assert.Empty(t, eval.Findings)
	assert.Equal(t, scaledToZeroGrace-time.Minute, eval.RecheckAfter)
}

func TestWorkloadIgnoresScaleToZeroWhenAnotherWorkloadServesTheService(
	t *testing.T,
) {
	m, api := zeroShop()
	slice := newID(kube.KindEndpointSlice, "shop", "api-1")
	put(m, slice, t0, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(2),
		kube.AttrEndpointsReady: inventory.Number(2),
	})
	link(m, slice, inventory.Backs, newID(kube.KindService, "shop", "api"))

	assert.Empty(t, detectZero(m, api), "blue/green twin still serves")
}

func TestWorkloadIgnoresScaleToZeroAnAutoscalerDisabled(t *testing.T) {
	m, api := zeroShop()
	hpa := newID(kube.KindHPA, "shop", "api")
	put(m, hpa, t0, conditionAttrs(map[string]inventory.Value{
		kube.AttrMinReplicas: inventory.Number(1)},
		"ScalingActive", "False", reasons.ScalingDisabled, t0))
	link(m, hpa, inventory.Scales, api)

	assert.Empty(t, detectZero(m, api))
}

// scaleDown records that a workload was set to zero replicas at at.
func scaleDown(m *inventory.Model, id inventory.EntityID, at time.Time) {
	m.Apply(inventory.Observation{
		Kind: inventory.Changed, Source: "test", At: at, Entity: id,
		Change: inventory.Change{Entity: id, At: at, Fields: []inventory.
			FieldChange{{Path: "spec.replicas", Before: "2", After: "0"}}},
	})
}

// Putting many workloads to sleep together is planned: the routes that
// are left over are not news.
func TestWorkloadScaledToZeroInAPlannedScaleDownIsLeftAlone(t *testing.T) {
	m, api := zeroShop()
	scaleDown(m, api, t0.Add(-9*time.Minute))
	for i := range kube.ScaleDownMin - 1 {
		other := newID(kube.KindDeployment, "shop", "other"+string(
			rune('a'+i)))
		put(m, other, t0.Add(-10*time.Minute), map[string]inventory.Value{
			kube.AttrReplicas: inventory.Number(0)})
		scaleDown(m, other, t0.Add(-time.Duration(8-i)*time.Minute))
	}

	assert.Empty(t, detectZero(m, api))
}

// A few workloads switched off are not a scale-down: still reported.
func TestWorkloadScaledToZeroWithFewOthersStillWarns(t *testing.T) {
	m, api := zeroShop()
	scaleDown(m, api, t0.Add(-9*time.Minute))
	for i := range kube.ScaleDownMin - 2 {
		other := newID(kube.KindDeployment, "shop", "other"+string(
			rune('a'+i)))
		put(m, other, t0.Add(-10*time.Minute), map[string]inventory.Value{
			kube.AttrReplicas: inventory.Number(0)})
		scaleDown(m, other, t0.Add(-8*time.Minute))
	}

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ScaledToZeroRouted, got[0].Reason)
}
