package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestClassifyRestartUsesPodAndNodeEvidence(t *testing.T) {
	oldPod, oldNode := os.Getenv("POD_NAME"), os.Getenv("NODE_NAME")
	t.Cleanup(func() {
		_ = os.Setenv("POD_NAME", oldPod)
		_ = os.Setenv("NODE_NAME", oldNode)
	})
	_ = os.Setenv("POD_NAME", "kwatch-new")
	_ = os.Setenv("NODE_NAME", "worker-a")
	previous := runtimeSession{
		SessionID: "previous", PodName: "kwatch-old",
		NodeName: "worker-a", StartedAt: time.Now(),
	}
	if got := classifyRestart(previous); got != "deployment_rollout" {
		t.Fatalf("classifyRestart() = %q", got)
	}
	previous.PodName = "kwatch-new"
	previous.NodeName = "worker-b"
	if got := classifyRestart(previous); got != "node_disruption" {
		t.Fatalf("classifyRestart() = %q", got)
	}
}

func TestClassifyRestartIgnoresCompletedSession(t *testing.T) {
	previous := runtimeSession{
		SessionID: "previous", EndedAt: time.Now(),
	}
	if got := classifyRestart(previous); got != "" {
		t.Fatalf("classifyRestart() = %q, want empty", got)
	}
}

func TestClassifyRestartUsesPersistedFailureCode(t *testing.T) {
	previous := runtimeSession{
		SessionID: "previous", FailureCode: "oom_killed",
	}
	if got := classifyRestart(previous); got != "oom_killed" {
		t.Fatalf("classifyRestart() = %q", got)
	}
}

func TestClassifyRestartUsesKubernetesEvidence(t *testing.T) {
	previous := runtimeSession{SessionID: "previous"}
	cases := []struct {
		name     string
		evidence restartEvidence
		want     string
	}{
		{name: "api", evidence: restartEvidence{APIUnavailable: true},
			want: "api_unavailable"},
		{name: "eviction", evidence: restartEvidence{PodReason: "Evicted"},
			want: "eviction"},
		{name: "node", evidence: restartEvidence{
			NodeObserved: true, NodeReason: "KubeletNotReady",
		}, want: "node_disruption"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := classifyWithEvidence(
				previous, testCase.evidence, "internal_failure",
			)
			if got != testCase.want {
				t.Fatalf("classifyWithEvidence() = %q, want %q", got,
					testCase.want)
			}
		})
	}
}

func TestStartupClaimKeyIsStable(t *testing.T) {
	first := startupClaimKey("v1", true, false, 0, "")
	second := startupClaimKey("v1", true, false, 0, "")
	if first != second {
		t.Fatalf("startup claim key is not stable: %q != %q", first, second)
	}
}

func TestNormalizeRestartReasonBoundsValues(t *testing.T) {
	if got := normalizeRestartReason("arbitrary"); got != "unknown" {
		t.Fatalf("normalizeRestartReason() = %q", got)
	}
}

type sessionStore struct {
	testStateStore
	session runtimeSession
}

func (s *sessionStore) GetRuntimeSession(
	context.Context,
) (runtimeSession, error) {
	return s.session, nil
}

func (s *sessionStore) SaveRuntimeSession(
	_ context.Context, value runtimeSession,
) error {
	s.session = value
	return nil
}

func TestStartupPersistsAndClosesRuntimeSession(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	store := &sessionStore{testStateStore: testStateStore{
		version: "dev", clusterID: "cluster-1",
	}}
	manager := newStartupManagerWithRuntime(
		store, config.RuntimeConfigFor(&config.Config{}),
		clock.RealClock{},
	)
	manager.now = func() time.Time { return now }
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if store.session.SessionID == "" {
		t.Fatal("Start() did not persist a session")
	}
	manager.EndSession(context.Background(), "graceful_shutdown")
	if store.session.EndReason != "graceful_shutdown" ||
		store.session.EndedAt.IsZero() {
		t.Fatalf("session was not closed: %+v", store.session)
	}
}

func TestRecordFailurePersistsBoundedEvidence(t *testing.T) {
	store := &sessionStore{testStateStore: testStateStore{
		version: "dev", clusterID: "cluster-1",
	}}
	manager := newStartupManagerWithRuntime(
		store, config.RuntimeConfigFor(&config.Config{}), clock.RealClock{},
	)
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	manager.RecordFailure(
		context.Background(),
		"component-with-a-name-longer-than-is-useful-for-diagnostics-"+
			"and-more",
		"unknown-code",
	)
	if store.session.FailureCode != "internal_failure" {
		t.Fatalf("failure code = %q", store.session.FailureCode)
	}
	if len(store.session.FailedComponent) > 64 {
		t.Fatalf("failure component length = %d", len(store.session.FailedComponent))
	}
}

// A rollout stops the old pod cleanly: the next start is a plain start,
// not "started again after an interrupted monitoring session".
func TestClassifyRestartTreatsGracefulShutdownAsClean(t *testing.T) {
	previous := runtimeSession{
		SessionID: "previous", EndedAt: time.Now(),
		EndReason: "graceful_shutdown",
	}
	if got := classifyRestart(previous); got != "" {
		t.Fatalf("classifyRestart() = %q, want a plain start", got)
	}
}
