package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// observeCustom describes a custom resource the way the dynamic source
// does and observes it.
func observeCustom(
	t *testing.T, m *inventory.Model, obj map[string]any,
) inventory.EntityID {
	t.Helper()
	u := &unstructured.Unstructured{Object: obj}
	group := u.GroupVersionKind().Group
	schema := kube.NewUnstructuredSchema(group, u.GetKind())
	desc, ok := schema.Describe(u)
	require.True(t, ok)
	put(m, desc.ID, t0, desc.Attributes)
	return desc.ID
}

func customObject(
	apiVersion, kind string, conditions ...map[string]any,
) map[string]any {
	list := make([]any, 0, len(conditions))
	for _, c := range conditions {
		c["lastTransitionTime"] = t0.Format(time.RFC3339)
		list = append(list, c)
	}
	return map[string]any{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]any{"name": "app", "namespace": "shop"},
		"status":   map[string]any{"conditions": list},
	}
}

func TestGenericCustomResourceFixtures(t *testing.T) {
	cases := []struct {
		name string
		obj  map[string]any
		mode string
	}{
		{"cert-manager certificate not ready", customObject(
			"cert-manager.io/v1", "Certificate", map[string]any{
				"type": "Ready", "status": "False",
				"reason": "IssuerNotReady", "message": "issuer missing",
			}), "Condition.Ready"},
		{"argo application degraded", customObject(
			"argoproj.io/v1alpha1", "Application", map[string]any{
				"type": "Degraded", "status": "True",
				"reason": "HealthDegraded",
			}), "Condition.Degraded"},
		{"flux kustomization stalled", customObject(
			"kustomize.toolkit.fluxcd.io/v1", "Kustomization",
			map[string]any{
				"type": "Stalled", "status": "True",
				"reason": "BuildFailed", "message": "kustomize build",
			}), "Condition.Stalled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel()
			id := observeCustom(t, m, tc.obj)
			now := t0.Add(DefaultConditionGrace)

			alone := evaluate(Generic{}, m, now, id, nil).Findings
			require.Len(t, alone, 1)
			assert.Equal(t, tc.mode, string(alone[0].Mode))

			// With the custom resource detector registered, the entity
			// is announced once, by the specialised detector.
			both := detection.NewRegistry(nil, Generic{}, Custom{}).
				Evaluate(m, now, id).Findings
			require.Len(t, both, 1)
			assert.Equal(t, reasons.CustomResourceFailure, both[0].Reason)
		})
	}
}

func TestGenericNestedGatewayCondition(t *testing.T) {
	obj := map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
		"metadata": map[string]any{"name": "web", "namespace": "shop"},
		"status": map[string]any{"parents": []any{map[string]any{
			"parentRef": map[string]any{"name": "edge"},
			"conditions": []any{map[string]any{
				"type": "ResolvedRefs", "status": "False",
				"reason":             "BackendNotFound",
				"message":            "service web missing",
				"lastTransitionTime": t0.Format(time.RFC3339),
			}},
		}}},
	}
	m := newTestModel()
	id := observeCustom(t, m, obj)
	got := evaluate(Generic{}, m, t0.Add(time.Hour), id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, "Condition.ResolvedRefs", string(got[0].Mode))
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "message", Value: "parent edge: service web missing",
	})
}

func TestGenericYieldsToSpecialisedKind(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPod, "shop", "api-1")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrPhase:         inventory.Text("Failed"),
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0),
		kube.AttrFinalizers:    inventory.Text("example.com/hold"),
	})
	setCondition(m, id, "Ready", "False", "", "", t0)
	registry := detection.NewRegistry(nil,
		Generic{}, NewPod(PodThresholds{}))
	got := registry.Evaluate(m, t0.Add(time.Hour), id).Findings
	for _, f := range got {
		assert.NotContains(t, f.Reason, reasons.ConditionFailure)
		assert.NotEqual(t, reasons.PhaseFailed, f.Reason)
		assert.NotEqual(t, reasons.StuckDeleting, f.Reason)
	}
}

func TestGenericDropsModeReportedBySpecialised(t *testing.T) {
	m, id := genericEntity(map[string]inventory.Value{
		kube.AttrCustom:      inventory.Bool(true),
		kube.AttrGeneration:  inventory.Number(2),
		kube.AttrObservedGen: inventory.Number(1),
	})
	setCondition(m, id, "Ready", "False", "Broken", "", t0)
	now := t0.Add(time.Hour)

	alone := evaluate(Generic{}, m, now, id, nil).Findings
	assert.ElementsMatch(t,
		[]string{"NotReconciling", "Condition.Ready"}, modes(alone))

	// Custom reports NotReconciling: the generic generation lag of the
	// same mode and the generic condition finding both yield.
	both := detection.NewRegistry(nil, Generic{}, Custom{}).
		Evaluate(m, now, id).Findings
	require.Len(t, both, 1)
	assert.Equal(t, reasons.CustomResourceFailure, both[0].Reason)
}

func TestGenericKeepsDistinctModes(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindDeployment, "shop", "api")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrGeneration:  inventory.Number(7),
		kube.AttrObservedGen: inventory.Number(6),
	})
	registry := detection.NewRegistry(nil, Generic{}, NewWorkload(0))
	got := registry.Evaluate(m, t0.Add(time.Hour), id).Findings
	assert.Contains(t, modes(got), "NotReconciling")
}
