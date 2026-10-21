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

// flakyDetector reports a finding and an onset until its panicAt-th call.
type flakyDetector struct {
	calls   *int
	panicAt int
	onset   *time.Time
}

func (flakyDetector) Name() string { return "flaky" }

func (flakyDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{"Pod"}
}

func (d flakyDetector) Detect(ctx Context, _ inventory.Entity) []Finding {
	*d.calls++
	if *d.calls == d.panicAt {
		panic("malformed object")
	}
	*d.onset = ctx.Onset("bad", ctx.Now)
	return []Finding{{Reason: "CrashLoopBackOff", Severity: Critical}}
}

func flakyModel(t *testing.T) (*inventory.Model, inventory.EntityID) {
	t.Helper()
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "p"}
	model := inventory.NewModel(inventory.Options{})
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test",
		At: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), Entity: id,
	})
	require.NoError(t, err)
	return model, id
}

func TestRegistryEvaluateNamesThePanickedDetector(t *testing.T) {
	calls, onset := 0, time.Time{}
	r := NewRegistry(nil, flakyDetector{&calls, 2, &onset})
	model, id := flakyModel(t)
	at := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	first := r.Evaluate(model, at, id)
	second := r.Evaluate(model, at.Add(time.Minute), id)

	require.Empty(t, first.Panicked)
	require.Equal(t, []string{"flaky"}, second.Panicked,
		"a detector that panicked left the entity half evaluated")
	require.Positive(t, second.RecheckAfter,
		"an incomplete evaluation asks to run again")
}

func TestRegistryKeepsOnsetsOfPanickedDetector(t *testing.T) {
	calls, onset := 0, time.Time{}
	r := NewRegistry(nil, flakyDetector{&calls, 2, &onset})
	model, id := flakyModel(t)
	at := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	r.Evaluate(model, at, id)
	r.Evaluate(model, at.Add(time.Minute), id)
	r.Evaluate(model, at.Add(2*time.Minute), id)

	require.True(t, onset.Equal(at),
		"onset = %v, want the first sighting %v: a panic must not "+
			"restart the wait", onset, at)
}

func TestRegistryCompleteEvaluationHasNoPanickedDetector(t *testing.T) {
	calls, onset := 0, time.Time{}
	r := NewRegistry(nil, flakyDetector{&calls, 0, &onset})
	model, id := flakyModel(t)

	eval := r.Evaluate(model, time.Now(), id)

	require.Empty(t, eval.Panicked)
}

// A finding names the detector that raised it, so a later panic of that
// detector can keep it.
func TestRegistryEvaluateNamesTheDetectorOfEachFinding(t *testing.T) {
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "p"}
	healthy := &mockDetectorWithFinding{
		name: "healthy", kinds: []inventory.Kind{"Pod"},
		findings: []Finding{{Reason: "NotReady", Severity: Warning}},
	}
	model := inventory.NewModel(inventory.Options{})
	at := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: at, Entity: id,
	})
	require.NoError(t, err)

	eval := NewRegistry(nil, healthy).Evaluate(model, at, id)

	require.Len(t, eval.Findings, 1)
	require.Equal(t, "healthy", eval.Findings[0].Detector)
}

func TestEvaluationKeepingAddsOnlyThePanickedDetectorsFindings(t *testing.T) {
	previous := []Finding{
		{Reason: "A-old", Detector: "a"}, {Reason: "B-old", Detector: "b"},
		{Reason: "A-fresh", Detector: "a"},
	}
	ev := Evaluation{
		Findings: []Finding{{Reason: "B-new", Detector: "b"},
			{Reason: "A-fresh", Detector: "other"}},
		Panicked: []string{"a"},
	}

	got := ev.Keeping(previous)

	var reasons []string
	for _, f := range got {
		reasons = append(reasons, f.Reason)
	}
	require.Equal(t, []string{"B-new", "A-fresh", "A-old"}, reasons,
		"b's cleared finding is not kept; a fresh finding replaces a "+
			"kept one of the same reason")
	require.Equal(t, ev.Findings[:1],
		Evaluation{Findings: ev.Findings[:1]}.Keeping(previous),
		"without a panic the fresh findings stand alone")
}
