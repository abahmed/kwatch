package detection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

type fallbackDetector struct {
	mockDetectorWithFinding
	specialised *bool
}

func (*fallbackDetector) Fallback() {}

func (f *fallbackDetector) Detect(
	ctx Context, _ inventory.Entity,
) []Finding {
	if f.specialised != nil {
		*f.specialised = ctx.Specialised()
	}
	return f.findings
}

func observed(kind inventory.Kind) (*inventory.Model, inventory.EntityID) {
	m := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: kind, Namespace: "ns", Name: "x"}
	m.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: time.Unix(0, 0),
		Entity: id, Attributes: map[string]inventory.Value{
			"a": inventory.Text("b"),
		},
	})
	return m, id
}

func reasonsOf(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Reason)
	}
	return out
}

func TestRegistryFallbackYieldsToSameMode(t *testing.T) {
	generic := fallbackDetector{
		mockDetectorWithFinding: mockDetectorWithFinding{
			kinds: []inventory.Kind{AnyKind},
			findings: []Finding{
				{Reason: "GenericLag", Mode: "NotReconciling"},
				{Reason: "GenericStuck", Mode: "StuckDeleting"},
				{Reason: "GenericReady", Mode: ConditionMode + "Ready"},
			},
		}}
	specialised := &mockDetectorWithFinding{
		kinds:    []inventory.Kind{AnyKind},
		findings: []Finding{{Reason: "Special", Mode: "NotReconciling"}},
	}
	m, id := observed("widget")

	got := NewRegistry(nil, &generic, specialised).Evaluate(m, time.Now(), id)
	assert.Equal(t, []string{"Special", "GenericStuck"},
		reasonsOf(got.Findings))

	alone := NewRegistry(nil, &generic).Evaluate(m, time.Now(), id)
	assert.Equal(t, []string{"GenericLag", "GenericStuck", "GenericReady"},
		reasonsOf(alone.Findings))
}

func TestRegistryReportsSpecialisedKind(t *testing.T) {
	var specialised bool
	generic := fallbackDetector{
		mockDetectorWithFinding: mockDetectorWithFinding{
			kinds: []inventory.Kind{AnyKind},
		},
		specialised: &specialised,
	}
	own := &mockDetector{kinds: []inventory.Kind{"pod"}}
	registry := NewRegistry(nil, &generic, own)

	m, id := observed("pod")
	registry.Evaluate(m, time.Now(), id)
	assert.True(t, specialised)

	m, id = observed("widget")
	registry.Evaluate(m, time.Now(), id)
	assert.False(t, specialised)
}
