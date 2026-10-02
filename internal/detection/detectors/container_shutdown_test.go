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

// terminatedIn builds a pod with podAttrs and one terminated container.
func terminatedIn(
	podAttrs map[string]inventory.Value, code float64, sidecar bool,
	reason string,
) (*inventory.Model, inventory.Entity) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model := newTestModel()
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "ns",
		Name: "p"}
	container := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "ns", Name: "p/c"}
	model.Apply(inventory.Observation{Kind: inventory.Observed, Source: "test",
		At: now, Entity: pod, Attributes: podAttrs})
	model.Apply(inventory.Observation{Kind: inventory.Observed, Source: "test",
		At: now, Entity: container, Attributes: map[string]inventory.Value{
			kube.AttrState:       inventory.Text("terminated"),
			kube.AttrExitCode:    inventory.Number(code),
			kube.AttrSidecar:     inventory.Bool(sidecar),
			kube.AttrStateReason: inventory.Text(reason),
		}})
	model.Apply(inventory.Observation{Kind: inventory.Related, Source: "test",
		At: now, Entity: container, Relation: inventory.PartOf,
		Targets: []inventory.EntityID{pod}})
	entity, _ := model.Entity(container)
	return model, entity
}

func phase(value string) map[string]inventory.Value {
	return map[string]inventory.Value{kube.AttrPhase: inventory.Text(value)}
}

func TestContainerStoppedOnPurposeIsNotAnError(t *testing.T) {
	cases := map[string]struct {
		pod     map[string]inventory.Value
		code    float64
		sidecar bool
	}{
		"succeeded pod": {phase("Succeeded"), 143, false},
		"deleting pod": {map[string]inventory.Value{
			kube.AttrPhase:    inventory.Text("Running"),
			kube.AttrDeleting: inventory.Bool(true),
		}, 143, false},
		"disruption target": {map[string]inventory.Value{
			kube.AttrPhase:                        inventory.Text("Failed"),
			kube.ConditionKey("DisruptionTarget"): inventory.Text("True"),
		}, 1, false},
		"sidecar at job end": {phase("Failed"), 143, true},
		"sidecar killed":     {phase("Running"), 137, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model, c := terminatedIn(tc.pod, tc.code, tc.sidecar, "Error")
			got := Container{}.Detect(
				testDetectorContext(model, time.Now()), c)
			assert.Empty(t, got)
		})
	}
}

func TestContainerRealTerminationsStillReported(t *testing.T) {
	cases := map[string]struct {
		pod     map[string]inventory.Value
		code    float64
		sidecar bool
		reason  string
		want    string
	}{
		"job failed": {phase("Failed"), 1, false, "Error",
			reasons.Error},
		"sigterm while running": {phase("Running"), 143, false, "Error",
			reasons.Error},
		"sidecar crash": {phase("Running"), 2, true, "Error",
			reasons.Error},
		"oom on deleting pod": {map[string]inventory.Value{
			kube.AttrDeleting: inventory.Bool(true),
		}, 137, false, reasons.OOMKilled, reasons.OOMKilled},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model, c := terminatedIn(tc.pod, tc.code, tc.sidecar,
				tc.reason)
			got := Container{}.Detect(
				testDetectorContext(model, time.Now()), c)
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].Reason)
		})
	}
}
