package detection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
)

type panickingDetector struct{ kinds []inventory.Kind }

func (panickingDetector) Name() string { return "panicking" }

func (d panickingDetector) Kinds() []inventory.Kind { return d.kinds }

func (panickingDetector) Detect(Context, inventory.Entity) []Finding {
	panic("malformed object")
}

func TestRegistryEvaluateContainsDetectorPanic(t *testing.T) {
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "p"}
	healthy := &mockDetectorWithFinding{
		name: "healthy", kinds: []inventory.Kind{"Pod"},
		findings: []Finding{{Reason: "NotReady", Severity: Warning}},
	}
	r := NewRegistry(nil,
		panickingDetector{kinds: []inventory.Kind{"Pod"}}, healthy)
	model := inventory.NewModel(inventory.Options{})
	at := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: at, Entity: id,
	})
	require.NoError(t, err)

	before := metrics.DefaultRegistry().PanicCount("detector")
	eval := r.Evaluate(model, at, id)

	require.Len(t, eval.Findings, 1, "other detectors still report")
	require.Equal(t, "NotReady", eval.Findings[0].Reason)
	require.Equal(t, before+1,
		metrics.DefaultRegistry().PanicCount("detector"))
}
