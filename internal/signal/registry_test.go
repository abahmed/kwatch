package signal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/knowledge"
)

type mockDetector struct {
	name  string
	kinds []knowledge.Kind
	calls int
}

func (m *mockDetector) Name() string {
	return m.name
}

func (m *mockDetector) Kinds() []knowledge.Kind {
	return m.kinds
}

func (m *mockDetector) Detect(ctx Context,
	entity knowledge.Entity) []Signal {
	m.calls++
	return []Signal{}
}

type mockDetectorWithSignal struct {
	name    string
	kinds   []knowledge.Kind
	signals []Signal
}

func (m *mockDetectorWithSignal) Name() string {
	return m.name
}

func (m *mockDetectorWithSignal) Kinds() []knowledge.Kind {
	return m.kinds
}

func (m *mockDetectorWithSignal) Detect(ctx Context,
	entity knowledge.Entity) []Signal {
	return m.signals
}

func TestNewRegistry(t *testing.T) {
	d1 := &mockDetector{name: "d1", kinds: []knowledge.Kind{"Pod"}}
	d2 := &mockDetector{name: "d2", kinds: []knowledge.Kind{"Node"}}

	r := NewRegistry(nil, d1, d2)
	assert.NotNil(t, r)
}

func TestRegistryEvaluateAbsentEntity(t *testing.T) {
	d := &mockDetectorWithSignal{
		name:  "test",
		kinds: []knowledge.Kind{"Pod"},
	}

	r := NewRegistry(nil, d)
	model := knowledge.NewModel(knowledge.Options{})
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	eval := r.Evaluate(model, time.Now(), id)
	assert.Empty(t, eval.Signals)
	assert.Equal(t, time.Duration(0), eval.RecheckAfter)
}

func TestRegistryEvaluatePresent(t *testing.T) {
	d := &mockDetectorWithSignal{
		name:  "test",
		kinds: []knowledge.Kind{"Pod"},
		signals: []Signal{
			{
				Reason:   "NotReady",
				Severity: Warning,
				Summary:  "Pod not ready",
			},
		},
	}

	r := NewRegistry(nil, d)
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := knowledge.NewModel(knowledge.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			"phase": knowledge.Text("Running"),
		},
	})

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	eval := r.Evaluate(model, now, id)

	require.Len(t, eval.Signals, 1)
	assert.Equal(t, "NotReady", eval.Signals[0].Reason)
	assert.Equal(t, id, eval.Signals[0].Entity)
	assert.Equal(t, now, eval.Signals[0].Since)
}

func TestRegistryEvaluateMultipleDetectors(t *testing.T) {
	d1 := &mockDetectorWithSignal{
		name:  "d1",
		kinds: []knowledge.Kind{"Pod"},
		signals: []Signal{
			{
				Reason:   "Signal1",
				Severity: Warning,
				Summary:  "First signal",
			},
		},
	}

	d2 := &mockDetectorWithSignal{
		name:  "d2",
		kinds: []knowledge.Kind{"Pod"},
		signals: []Signal{
			{
				Reason:   "Signal2",
				Severity: Critical,
				Summary:  "Second signal",
			},
		},
	}

	r := NewRegistry(nil, d1, d2)
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := knowledge.NewModel(knowledge.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			"phase": knowledge.Text("Running"),
		},
	})

	eval := r.Evaluate(model, time.Now(), id)

	require.Len(t, eval.Signals, 2)
	assert.Equal(t, "Signal1", eval.Signals[0].Reason)
	assert.Equal(t, "Signal2", eval.Signals[1].Reason)
}

func TestRegistryEvaluateSetsEntity(t *testing.T) {
	d := &mockDetectorWithSignal{
		name:  "test",
		kinds: []knowledge.Kind{"Pod"},
		signals: []Signal{
			{
				Reason:   "Test",
				Severity: Warning,
				Summary:  "Test",
			},
		},
	}

	r := NewRegistry(nil, d)
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := knowledge.NewModel(knowledge.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			"phase": knowledge.Text("Running"),
		},
	})

	eval := r.Evaluate(model, time.Now(), id)

	require.Len(t, eval.Signals, 1)
	assert.Equal(t, id, eval.Signals[0].Entity)
}

func TestRegistryEvaluateSetsSince(t *testing.T) {
	d := &mockDetectorWithSignal{
		name:  "test",
		kinds: []knowledge.Kind{"Pod"},
		signals: []Signal{
			{
				Reason:   "Test",
				Severity: Warning,
				Summary:  "Test",
			},
		},
	}

	r := NewRegistry(nil, d)
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := knowledge.NewModel(knowledge.Options{})
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			"phase": knowledge.Text("Running"),
		},
	})

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	eval := r.Evaluate(model, now, id)

	require.Len(t, eval.Signals, 1)
	assert.Equal(t, now, eval.Signals[0].Since)
}

func TestRegistryEvaluatePreservesExplicitSince(t *testing.T) {
	explicitSince := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	d := &mockDetectorWithSignal{
		name:  "test",
		kinds: []knowledge.Kind{"Pod"},
		signals: []Signal{
			{
				Reason:   "Test",
				Severity: Warning,
				Summary:  "Test",
				Since:    explicitSince,
			},
		},
	}

	r := NewRegistry(nil, d)
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "test"}

	model := knowledge.NewModel(knowledge.Options{})
	t0 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     t0,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			"phase": knowledge.Text("Running"),
		},
	})

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	eval := r.Evaluate(model, now, id)

	require.Len(t, eval.Signals, 1)
	assert.Equal(t, explicitSince, eval.Signals[0].Since)
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
		synced: func(k knowledge.Kind) bool {
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
