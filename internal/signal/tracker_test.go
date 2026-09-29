package signal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/knowledge"
)

func TestNewTrackerEmpty(t *testing.T) {
	tr := NewTracker()
	assert.NotNil(t, tr)

	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}
	assert.False(t, tr.Has(id))
	assert.Empty(t, tr.Active(id))
}

func TestObserveRaises(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	signal := Signal{
		Entity: id, Reason: "NotReady", Severity: Warning,
		Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary: "Pod not ready",
	}

	transitions := tr.Observe(id, []Signal{signal})

	require.Len(t, transitions, 1)
	assert.Equal(t, Raised, transitions[0].Kind)
	assert.Equal(t, signal.Reason, transitions[0].Signal.Reason)
	assert.True(t, tr.Has(id))
}

func TestObservePreservesChronology(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}
	firstSeen := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// First observation
	signal := Signal{
		Entity: id, Reason: "NotReady", Severity: Warning,
		Since:   firstSeen,
		Summary: "Pod not ready",
	}
	tr.Observe(id, []Signal{signal})

	// Re-observation with same signal but later time
	signal.Since = time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)
	transitions := tr.Observe(id, []Signal{signal})

	// Should preserve original Since
	require.Len(t, transitions, 0)

	active := tr.Active(id)
	require.Len(t, active, 1)
	assert.Equal(t, firstSeen, active[0].Since)
}

func TestObserveChangedOnSeverityChange(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	// First observation
	signal := Signal{
		Entity: id, Reason: "NotReady", Severity: Warning,
		Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary: "Pod not ready",
	}
	tr.Observe(id, []Signal{signal})

	// Change severity
	signal.Severity = Critical
	transitions := tr.Observe(id, []Signal{signal})

	require.Len(t, transitions, 1)
	assert.Equal(t, Changed, transitions[0].Kind)
	assert.Equal(t, Critical, transitions[0].Signal.Severity)
}

func TestObserveChangedOnSummaryChange(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	// First observation
	signal := Signal{
		Entity: id, Reason: "NotReady", Severity: Warning,
		Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary: "Pod not ready",
	}
	tr.Observe(id, []Signal{signal})

	// Change summary
	signal.Summary = "Pod is still not ready after 5 minutes"
	transitions := tr.Observe(id, []Signal{signal})

	require.Len(t, transitions, 1)
	assert.Equal(t, Changed, transitions[0].Kind)
	assert.Equal(t, signal.Summary, transitions[0].Signal.Summary)
}

func TestObserveNoChangeOnIdentialSignal(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	signal := Signal{
		Entity: id, Reason: "NotReady", Severity: Warning,
		Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary: "Pod not ready",
	}

	tr.Observe(id, []Signal{signal})
	transitions := tr.Observe(id, []Signal{signal})

	assert.Empty(t, transitions)
}

func TestObserveClears(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	signal := Signal{
		Entity: id, Reason: "NotReady", Severity: Warning,
		Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary: "Pod not ready",
	}

	tr.Observe(id, []Signal{signal})
	transitions := tr.Observe(id, []Signal{})

	require.Len(t, transitions, 1)
	assert.Equal(t, Cleared, transitions[0].Kind)
	assert.False(t, tr.Has(id))
}

func TestObserveMultipleSignals(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	signals := []Signal{
		{
			Entity: id, Reason: "NotReady", Severity: Warning,
			Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary: "Pod not ready",
		},
		{
			Entity: id, Reason: "ImagePullBackOff", Severity: Critical,
			Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary: "Failed to pull image",
		},
	}

	transitions := tr.Observe(id, signals)

	// Should have 2 Raised transitions in sorted order
	require.Len(t, transitions, 2)
	assert.Equal(t, Raised, transitions[0].Kind)
	assert.Equal(t, Raised, transitions[1].Kind)
	assert.Equal(t, "ImagePullBackOff", transitions[0].Signal.Reason)
	assert.Equal(t, "NotReady", transitions[1].Signal.Reason)
}

func TestTransitionsDeterministicOrder(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	// First batch
	signals1 := []Signal{
		{
			Entity: id, Reason: "Zebra", Severity: Warning,
			Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary: "z",
		},
		{
			Entity: id, Reason: "Apple", Severity: Warning,
			Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary: "a",
		},
		{
			Entity: id, Reason: "Monkey", Severity: Warning,
			Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary: "m",
		},
	}

	transitions := tr.Observe(id, signals1)

	// Should be sorted by reason
	reasons := []string{
		transitions[0].Signal.Reason,
		transitions[1].Signal.Reason,
		transitions[2].Signal.Reason,
	}
	assert.Equal(t, []string{"Apple", "Monkey", "Zebra"}, reasons)
}

func TestActiveSorted(t *testing.T) {
	tr := NewTracker()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "test"}

	signals := []Signal{
		{
			Entity: id, Reason: "Zebra", Severity: Warning,
			Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary: "z",
		},
		{
			Entity: id, Reason: "Apple", Severity: Warning,
			Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary: "a",
		},
	}

	tr.Observe(id, signals)
	active := tr.Active(id)

	require.Len(t, active, 2)
	assert.Equal(t, "Apple", active[0].Reason)
	assert.Equal(t, "Zebra", active[1].Reason)
}

func TestMultipleEntities(t *testing.T) {
	tr := NewTracker()
	id1 := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "pod1"}
	id2 := knowledge.EntityID{Kind: "Pod", Namespace: "default",
		Name: "pod2"}

	signal1 := Signal{
		Entity: id1, Reason: "NotReady", Severity: Warning,
		Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary: "Pod1 not ready",
	}

	signal2 := Signal{
		Entity: id2, Reason: "NotReady", Severity: Warning,
		Since:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary: "Pod2 not ready",
	}

	tr.Observe(id1, []Signal{signal1})
	tr.Observe(id2, []Signal{signal2})

	assert.True(t, tr.Has(id1))
	assert.True(t, tr.Has(id2))

	active1 := tr.Active(id1)
	require.Len(t, active1, 1)
	assert.Equal(t, "Pod1 not ready", active1[0].Summary)

	active2 := tr.Active(id2)
	require.Len(t, active2, 1)
	assert.Equal(t, "Pod2 not ready", active2[0].Summary)
}
