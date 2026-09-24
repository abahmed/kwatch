package insight

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

func TestCauseCandidatesPreferRecentActiveDependency(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "apps", "api", "configmap", "apps", "old", "mounts")
	graph.AddEdge("pod", "apps", "api", "secret", "apps", "new", "env_from")
	tracker := context.NewChangeTrackerWithClock(10, clock.Func(func() time.Time {
		return now
	}))
	tracker.Record(context.Change{
		Resource: "configmap", Namespace: "apps", Name: "old",
		Type: context.ChangeUpdate, Timestamp: now.Add(-9 * time.Minute),
	})
	tracker.Record(context.Change{
		Resource: "secret", Namespace: "apps", Name: "new",
		Type: context.ChangeUpdate, Timestamp: now.Add(-30 * time.Second),
	})
	e := NewEngineWithDependencies(graph, tracker, Dependencies{
		Clock: clock.Func(func() time.Time { return now }),
		ActiveChecker: func(kind, namespace, name string) bool {
			return kind == "secret" && namespace == "apps" && name == "new"
		},
	})
	inc := &model.Incident{Subject: model.Subject{
		Key: "pod/apps/api", Resource: "pod", Namespace: "apps", Name: "api",
	}}

	candidates := e.causeCandidates(inc, &Insight{})
	require.NotEmpty(t, candidates)
	require.Equal(t, "secret", candidates[0].Ref.Kind)
	require.Equal(t, "new", candidates[0].Ref.Name)
	require.Greater(t, candidates[0].Score, candidates[1].Score)
}

func TestShouldAnnounceReevaluationOnlyWhenDiagnosisChanges(t *testing.T) {
	e := newTestEngine(nil, nil)
	inc := &model.Incident{Subject: model.Subject{Key: "pod/apps/api"}}
	first := &Insight{
		CauseState: CauseLikely,
		RootCause:  model.ObjectRef{Kind: "node", Name: "n1"},
		Pattern:    "node_failure", Impact: "one pod",
		Severity: model.SeverityWarning,
	}
	if !e.ShouldAnnounceReevaluation(inc, first) {
		t.Fatal("first diagnosis was not announced")
	}
	if e.ShouldAnnounceReevaluation(inc, first) {
		t.Fatal("unchanged diagnosis was announced twice")
	}
	changed := *first
	changed.RootCause = model.ObjectRef{Kind: "node", Name: "n2"}
	changed.Impact = "three pods"
	changed.Severity = model.SeverityCritical
	if !e.ShouldAnnounceReevaluation(inc, &changed) {
		t.Fatal("changed diagnosis was suppressed")
	}
}

func TestRolloutSuppressionExpiresAtTenMinutes(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	tracker := context.NewChangeTrackerWithClock(10, clock.Func(func() time.Time {
		return now
	}))
	tracker.Record(context.Change{
		Resource: "deployment", Namespace: "apps", Name: "api",
		Type: context.ChangeUpdate, Timestamp: now.Add(-time.Minute),
	})
	e := NewEngineWithDependencies(nil, tracker, Dependencies{
		Clock: clock.Func(func() time.Time { return now }),
	})
	inc := &model.Incident{Subject: model.Subject{
		Key: "pod/apps/api", Resource: "pod", Namespace: "apps", Name: "api",
		OwnerKind: "Deployment",
	}, Status: model.Status{Severity: model.SeverityWarning},
		Evidence: model.Evidence{
			Facts: model.Facts{DesiredReplicas: 2, ReadyReplicas: 1},
		}}

	ins := &Insight{
		Pattern: "rollout", Maintenance: "a Deployment rollout started",
	}
	e.applyRolloutSuppression(inc, ins)
	require.Equal(t, "expected_rollout_with_capacity", ins.SuppressReason)

	now = now.Add(rolloutGrace)
	ins = &Insight{
		Pattern: "rollout", Maintenance: "a Deployment rollout started",
	}
	e.applyRolloutSuppression(inc, ins)
	require.Empty(t, ins.SuppressReason)
	require.Contains(
		t, ins.Evidence,
		"the rollout remained degraded beyond the 10m grace period",
	)
}

func TestRolloutRecoveryResetsSuppressionWindow(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	tracker := context.NewChangeTrackerWithClock(10, clock.Func(func() time.Time {
		return now
	}))
	tracker.Record(context.Change{
		Resource: "deployment", Namespace: "apps", Name: "api",
		Type: context.ChangeUpdate, Timestamp: now,
	})
	e := NewEngineWithDependencies(nil, tracker, Dependencies{
		Clock: clock.Func(func() time.Time { return now }),
	})
	inc := &model.Incident{Subject: model.Subject{
		Key: "pod/apps/api", Resource: "pod", Namespace: "apps", Name: "api",
		OwnerKind: "Deployment",
	}, Evidence: model.Evidence{Facts: model.Facts{
		DesiredReplicas: 2, ReadyReplicas: 1,
	}}}
	e.Analyze(inc)
	if e.states[inc.Key].suppressed.IsZero() {
		t.Fatal("rollout suppression was not recorded")
	}

	inc.Facts.ReadyReplicas = 0
	e.Analyze(inc)
	if !e.states[inc.Key].suppressed.IsZero() {
		t.Fatal("recovery did not clear rollout suppression")
	}

	inc.Facts.ReadyReplicas = 1
	now = now.Add(time.Minute)
	e.Analyze(inc)
	require.Equal(t, now, e.states[inc.Key].suppressed)
}

type captureClassifier struct{ input string }

func (c *captureClassifier) Classify(logs string) *LogSignal {
	c.input = logs
	return &LogSignal{Class: "connection", Summary: "connections failed"}
}

func TestLogClassificationUsesBoundedTail(t *testing.T) {
	classifier := &captureClassifier{}
	e := NewEngineWithDependencies(nil, nil, Dependencies{
		Clock:         clock.RealClock{},
		LogClassifier: classifier,
	})
	inc := &model.Incident{
		Subject: model.Subject{Key: "pod/apps/api"},
		Evidence: model.Evidence{
			IncludeLogs: true,
			Logs:        strings.Repeat("a", maxClassifierBytes) + "tail",
		},
	}

	ins := e.Analyze(inc)
	require.NotNil(t, ins.LogSignal)
	require.Len(t, classifier.input, maxClassifierBytes)
	require.True(t, strings.HasSuffix(classifier.input, "tail"))
}

func TestAnalysisStateBoundsTransitionsAndResetsOnRecovery(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	e := NewEngineWithDependencies(nil, nil, Dependencies{
		Clock: clock.Func(func() time.Time { return now }),
	})
	inc := &model.Incident{Subject: model.Subject{Key: "pod/apps/api"}}
	for i := 0; i < maxTransitions+5; i++ {
		e.ObserveOutcome(inc, model.IncidentAction(i%4), "test")
		now = now.Add(time.Minute)
	}
	if got := len(e.states[inc.Key].transitions); got != maxTransitions {
		t.Fatalf("transition history = %d, want %d", got, maxTransitions)
	}
	e.ObserveOutcome(inc, model.ActionResolved, "test")
	state := e.states[inc.Key]
	if !state.suppressed.IsZero() || state.diagnosis != "" || state.delivered {
		t.Fatal("resolution did not reset recurring analysis state")
	}
}

func TestDeliveryActionCreatesFirstEventualNotificationAndResets(t *testing.T) {
	e := newTestEngine(nil, nil)
	inc := &model.Incident{Subject: model.Subject{Key: "pod/apps/api"}}
	if got := e.DeliveryAction(inc, model.ActionUpdate); got !=
		model.ActionCreate {
		t.Fatalf("first eventual action = %v, want create", got)
	}
	e.RecordDelivery(inc)
	if got := e.DeliveryAction(inc, model.ActionUpdate); got !=
		model.ActionUpdate {
		t.Fatalf("delivered action = %v, want update", got)
	}
	e.ObserveOutcome(inc, model.ActionResolved, "test")
	if got := e.DeliveryAction(inc, model.ActionUpdate); got !=
		model.ActionCreate {
		t.Fatalf("recurrence action = %v, want create", got)
	}
}
