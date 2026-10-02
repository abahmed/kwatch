package detection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

type mockDetector struct {
	name  string
	kinds []inventory.Kind
	calls int
}

func (m *mockDetector) Name() string {
	return m.name
}

func (m *mockDetector) Kinds() []inventory.Kind {
	return m.kinds
}

func (m *mockDetector) Detect(ctx Context,
	entity inventory.Entity) []Finding {
	m.calls++
	return []Finding{}
}

type mockDetectorWithFinding struct {
	name     string
	kinds    []inventory.Kind
	findings []Finding
}

func (m *mockDetectorWithFinding) Name() string {
	return m.name
}

func (m *mockDetectorWithFinding) Kinds() []inventory.Kind {
	return m.kinds
}

func (m *mockDetectorWithFinding) Detect(ctx Context,
	entity inventory.Entity) []Finding {
	return m.findings
}

func TestNewRegistry(t *testing.T) {
	d1 := &mockDetector{name: "d1", kinds: []inventory.Kind{"Pod"}}
	d2 := &mockDetector{name: "d2", kinds: []inventory.Kind{"Node"}}

	r := NewRegistry(nil, d1, d2)
	assert.NotNil(t, r)
}

func TestRegistryEvaluateAbsentEntity(t *testing.T) {
	d := &mockDetectorWithFinding{
		name:  "test",
		kinds: []inventory.Kind{"Pod"},
	}

	r := NewRegistry(nil, d)
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	eval := r.Evaluate(model, time.Now(), id)
	assert.Empty(t, eval.Findings)
	assert.Equal(t, time.Duration(0), eval.RecheckAfter)
}

func TestRegistryEvaluatePresent(t *testing.T) {
	d := &mockDetectorWithFinding{
		name:  "test",
		kinds: []inventory.Kind{"Pod"},
		findings: []Finding{
			{
				Reason:   "NotReady",
				Severity: Warning,
				Summary:  "Pod not ready",
			},
		},
	}

	r := NewRegistry(nil, d)
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := inventory.NewModel(inventory.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]inventory.Value{
			"phase": inventory.Text("Running"),
		},
	})

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	eval := r.Evaluate(model, now, id)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "NotReady", eval.Findings[0].Reason)
	assert.Equal(t, id, eval.Findings[0].Entity)
	assert.Equal(t, now, eval.Findings[0].Since)
}

func TestRegistryEvaluateMultipleDetectors(t *testing.T) {
	d1 := &mockDetectorWithFinding{
		name:  "d1",
		kinds: []inventory.Kind{"Pod"},
		findings: []Finding{
			{
				Reason:   "Finding1",
				Severity: Warning,
				Summary:  "First finding",
			},
		},
	}

	d2 := &mockDetectorWithFinding{
		name:  "d2",
		kinds: []inventory.Kind{"Pod"},
		findings: []Finding{
			{
				Reason:   "Finding2",
				Severity: Critical,
				Summary:  "Second finding",
			},
		},
	}

	r := NewRegistry(nil, d1, d2)
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := inventory.NewModel(inventory.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]inventory.Value{
			"phase": inventory.Text("Running"),
		},
	})

	eval := r.Evaluate(model, time.Now(), id)

	require.Len(t, eval.Findings, 2)
	assert.Equal(t, "Finding1", eval.Findings[0].Reason)
	assert.Equal(t, "Finding2", eval.Findings[1].Reason)
}

func TestRegistryEvaluateSetsEntity(t *testing.T) {
	d := &mockDetectorWithFinding{
		name:  "test",
		kinds: []inventory.Kind{"Pod"},
		findings: []Finding{
			{
				Reason:   "Test",
				Severity: Warning,
				Summary:  "Test",
			},
		},
	}

	r := NewRegistry(nil, d)
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := inventory.NewModel(inventory.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]inventory.Value{
			"phase": inventory.Text("Running"),
		},
	})

	eval := r.Evaluate(model, time.Now(), id)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, id, eval.Findings[0].Entity)
}

func TestRegistryEvaluateSetsSince(t *testing.T) {
	d := &mockDetectorWithFinding{
		name:  "test",
		kinds: []inventory.Kind{"Pod"},
		findings: []Finding{
			{
				Reason:   "Test",
				Severity: Warning,
				Summary:  "Test",
			},
		},
	}

	r := NewRegistry(nil, d)
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := inventory.NewModel(inventory.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]inventory.Value{
			"phase": inventory.Text("Running"),
		},
	})

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	eval := r.Evaluate(model, now, id)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, now, eval.Findings[0].Since)
}

func TestRegistryEvaluatePreservesExplicitSince(t *testing.T) {
	explicitSince := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	d := &mockDetectorWithFinding{
		name:  "test",
		kinds: []inventory.Kind{"Pod"},
		findings: []Finding{
			{
				Reason:   "Test",
				Severity: Warning,
				Summary:  "Test",
				Since:    explicitSince,
			},
		},
	}

	r := NewRegistry(nil, d)
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := inventory.NewModel(inventory.Options{})
	t0 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]inventory.Value{
			"phase": inventory.Text("Running"),
		},
	})

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	eval := r.Evaluate(model, now, id)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, explicitSince, eval.Findings[0].Since)
}

func TestContextSyncedDefaultTrue(t *testing.T) {
	ctx := Context{Model: nil, Now: time.Now()}

	// When synced is nil, defaults to true
	assert.True(t, ctx.Synced("Pod"))
}

func TestContextSyncedCustom(t *testing.T) {
	ctx := Context{
		Model: nil,
		Now:   time.Now(),
		synced: func(k inventory.Kind) bool {
			return k == "Pod"
		},
	}

	assert.True(t, ctx.Synced("Pod"))
	assert.False(t, ctx.Synced("Node"))
}

func TestContextRecheckAfter(t *testing.T) {
	var recheck time.Duration
	ctx := Context{
		Model:   nil,
		Now:     time.Now(),
		recheck: &recheck,
	}

	delay := 5 * time.Second
	ctx.RecheckAfter(delay)

	assert.Equal(t, delay, recheck)
}

func TestContextRecheckAfterEarliest(t *testing.T) {
	var recheck time.Duration
	ctx := Context{
		Model:   nil,
		Now:     time.Now(),
		recheck: &recheck,
	}

	ctx.RecheckAfter(10 * time.Second)
	ctx.RecheckAfter(5 * time.Second)

	// Should keep the earliest
	assert.Equal(t, 5*time.Second, recheck)
}

func TestContextRecheckAfterIgnoresZero(t *testing.T) {
	var recheck = 10 * time.Second
	ctx := Context{
		Model:   nil,
		Now:     time.Now(),
		recheck: &recheck,
	}

	ctx.RecheckAfter(0)

	// Should not change
	assert.Equal(t, 10*time.Second, recheck)
}

// TestRegistryCollapsesDuplicateReasons: two detectors reporting the
// same reason for one entity yield one finding, the most serious,
// because the tracker keys findings by entity and reason.
func TestRegistryCollapsesDuplicateReasons(t *testing.T) {
	m, id := observed("widget")
	low := &mockDetectorWithFinding{name: "a", kinds: []inventory.Kind{"widget"},
		findings: []Finding{
			{Reason: "R", Severity: Info, Summary: "first"},
			{Reason: "Other", Severity: Info},
		}}
	high := &mockDetectorWithFinding{name: "b", kinds: []inventory.Kind{"widget"},
		findings: []Finding{
			{Reason: "R", Severity: Critical, Summary: "second"},
			{Reason: "R", Severity: Critical, Summary: "third"},
		}}

	got := NewRegistry(nil, low, high).Evaluate(m, time.Now(), id)

	assert.Equal(t, []string{"R", "Other"}, reasonsOf(got.Findings))
	assert.Equal(t, Critical, got.Findings[0].Severity)
	assert.Equal(t, "second", got.Findings[0].Summary)
}
