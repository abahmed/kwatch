package insight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	context "github.com/abahmed/kwatch/internal/graphcontext"
)

func TestMassFailureDescribe(t *testing.T) {
	mf := MassFailure{
		SharedDependency: "node//n1",
		AffectedCount:    5,
		Threshold:        3,
		Reason:           "CrashLoopBackOff",
		Namespace:        "ns",
		ResourceKind:     "pod",
	}
	desc := mf.Describe()
	assert.Contains(t, desc, "5")
	assert.Contains(t, desc, "pod")
	assert.Contains(t, desc, "/n1")
}

func TestMassFailureDescribeEnriched(t *testing.T) {
	mf := MassFailure{
		SharedDependency: "configmap/ns/cm1",
		AffectedCount:    4,
		Threshold:        3,
		Reason:           "CrashLoopBackOff",
		Namespace:        "ns",
		ResourceKind:     "pod",
		RootCause: "underlying node n1 may be unhealthy (pattern: " +
			"node_failure)",
		RecentChanges: []context.Change{
			{
				Resource:  "configmap",
				Namespace: "ns",
				Name:      "cm1",
				Type:      context.ChangeUpdate,
				Timestamp: time.Now().Add(-2 * time.Minute),
			},
		},
	}
	desc := mf.Describe()
	assert.Contains(t, desc, "root cause: underlying node n1")
	assert.Contains(t, desc, "recent changes")
	assert.Contains(t, desc, "cm1")
}

func TestMassFailureDescribeClampsFutureTimestamps(t *testing.T) {
	mf := MassFailure{
		SharedDependency: "configmap/ns/cm1",
		AffectedCount:    4,
		Threshold:        3,
		Reason:           "CrashLoopBackOff",
		Namespace:        "ns",
		ResourceKind:     "pod",
		RecentChanges: []context.Change{
			// Future timestamp (clock skew): must render as 0s, never "-Xs".
			{
				Resource:  "configmap",
				Namespace: "ns",
				Name:      "cm1",
				Type:      context.ChangeUpdate,
				Timestamp: time.Now().Add(5 * time.Minute),
			},
		},
	}
	desc := mf.Describe()
	assert.Contains(t, desc, "recent changes: ns/cm1 updated 0s ago")
	assert.NotContains(t, desc, "-")
}

func TestEnrichMassFailureRootCause(t *testing.T) {
	graph := context.NewResourceGraph()
	// configmap/ns/cm1 <- pod p1. cm1 is a leaf, so walking up from cm1 lands
	// on cm1 itself as the root.
	graph.AddEdge("pod", "ns", "p1", "configmap", "ns", "cm1", "mounts")

	tracker := context.NewChangeTracker(0)
	tracker.Record(context.Change{
		Resource:  "configmap",
		Namespace: "ns",
		Name:      "cm1",
		Type:      context.ChangeUpdate,
		Timestamp: time.Now().Add(-30 * time.Second),
	})

	e := NewEngine(graph, tracker)
	mf := e.EnrichMassFailure(MassFailure{
		SharedDependency: "configmap/ns/cm1",
		AffectedCount:    4,
		Threshold:        3,
	})
	assert.Contains(
		t,
		mf.RootCause,
		"underlying configmap cm1 may be changed or misconfigured",
	)
	assert.Contains(t, mf.RootCause, "config_error")
	assert.Len(t, mf.RecentChanges, 1)
	assert.Equal(t, "cm1", mf.RecentChanges[0].Name)
}

func TestEnrichMassFailureDeepRoot(t *testing.T) {
	graph := context.NewResourceGraph()
	// pv-1 <- node n1 and pv-1 <- pod p1; the shared dep is pv-1, whose
	// upstream
	// is the node — the walker should surface n1 as the root cause.
	graph.AddEdge("persistentvolume", "", "pv-1", "node", "", "n1", "local_at")
	graph.AddEdge("pod", "ns", "p1", "persistentvolume", "", "pv-1", "binds")

	e := NewEngine(graph, context.NewChangeTracker(0))
	mf := e.EnrichMassFailure(MassFailure{
		SharedDependency: "persistentvolume//pv-1",
		AffectedCount:    4,
		Threshold:        3,
	})
	assert.Contains(t, mf.RootCause, "underlying node n1 may be unhealthy")
	assert.Contains(t, mf.RootCause, "node_failure")
}
