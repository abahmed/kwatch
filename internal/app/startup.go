package app

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/version"
)

// startupStateStore is the startup subset of persistence. It keeps the
// startup session independent of the full storage manager and makes the
// lifecycle state flow explicit.
type startupStateStore interface {
	EnsureClusterID(context.Context) (string, error)
	IsFirstRun(context.Context) (bool, error)
	GetStoredVersion(context.Context) (string, error)
	MarkAsInitialized(context.Context, string, string) error
	GetLastSeen(context.Context) (time.Time, error)
	SetLastSeen(context.Context, time.Time) error
}

type startupAnnouncementStore interface {
	ClaimStartupAnnouncement(context.Context, string) (bool, error)
}

type runtimeSessionStore interface {
	GetRuntimeSession(context.Context) (runtimeSession, error)
	SaveRuntimeSession(context.Context, runtimeSession) error
}

// restartEvidence is the bounded evidence used to explain an incomplete
// monitoring session. The startup session does not read Kubernetes itself;
// a restartEvidenceSource supplies this value.
type restartEvidence struct {
	PodReason       string
	ContainerReason string
	NodeObserved    bool
	NodeReady       bool
	NodeReason      string
	LeaseLost       bool
	APIUnavailable  bool
}

// restartEvidenceSource adapts Kubernetes state to the restart
// classification contract. Implementations must return bounded fields and
// may report no evidence when the previous object has already been removed.
type restartEvidenceSource interface {
	ReadRestartEvidence(
		context.Context, runtimeSession,
	) (restartEvidence, error)
}

type startupManager struct {
	persistenceManager    startupStateStore
	disableStartupMessage bool
	shouldNotify          bool
	// downtime is how long monitoring was unavailable before this start,
	// zero when there is no previous record or the gap was insignificant.
	downtime       time.Duration
	currentVersion string
	// sessionMu guards session: the alive loop and the shutdown path may
	// touch it while startup is still finishing.
	sessionMu      sync.Mutex
	session        runtimeSession
	restartReason  string
	evidenceSource restartEvidenceSource
	now            func() time.Time
	// stateReset records that the state file was replaced at open, so
	// the startup message can say why old problems are announced again.
	stateReset bool
}

// startupResult is what application composition needs from the startup
// decision: the cluster identity and the running version. Whether and
// what to announce stays with the startup manager (StartupMessage).
type startupResult struct {
	ClusterID      string
	CurrentVersion string
}

// newStartupManagerWithRuntime constructs startup from the immutable runtime
// snapshot used by application composition.
func newStartupManagerWithRuntime(
	state startupStateStore,
	runtime config.RuntimeConfig,
	now clock.Clock,
	evidence ...restartEvidenceSource,
) *startupManager {
	manager := newStartupManager(
		state, runtime.Application().DisableStartupMessage, now,
	)
	if len(evidence) > 0 {
		manager.evidenceSource = evidence[0]
	}
	return manager
}

func newStartupManager(
	state startupStateStore,
	disableStartupMessage bool,
	now clock.Clock,
) *startupManager {
	now = clock.Require(now)
	sm := &startupManager{
		persistenceManager:    state,
		disableStartupMessage: disableStartupMessage,
		now:                   now.Now,
	}
	return sm
}

// storeResetReporter is the part of persistence that knows whether the
// state file was replaced because it could not be used.
type storeResetReporter interface {
	StoreWasReset() bool
}

// StoreWasReset reports whether opening the state file replaced it.
func (d diskState) StoreWasReset() bool {
	if d.store == nil {
		return false
	}
	_, reset := d.store.Reset()
	return reset
}

// decideAnnouncement sets shouldNotify: whether this start deserves a
// startup message, and whether this replica won the right to send it.
// A reset state file is always announced: without it, operators would
// see old problems announced again with no reason given.
func (s *startupManager) decideAnnouncement(
	ctx context.Context, isFirstRun, isUpgrade bool,
) error {
	s.shouldNotify = (isFirstRun || isUpgrade || s.downtime > 0 ||
		s.stateReset) && !s.disableStartupMessage
	if s.restartReason == "internal_failure" &&
		!s.disableStartupMessage {
		s.shouldNotify = true
	}
	if !s.shouldNotify {
		return nil
	}
	claimed, err := s.claimStartupAnnouncement(ctx, isFirstRun, isUpgrade)
	if err != nil {
		return fmt.Errorf("claim startup announcement: %w", err)
	}
	s.shouldNotify = claimed
	return nil
}

// Start loads and records startup state, returning the decision needed by the
// application lifecycle. State needed to fence a new active generation is
// fail-closed: an API or RBAC error must not look like a first run.
func (s *startupManager) Start(ctx context.Context) (startupResult, error) {
	clusterID, err := s.persistenceManager.EnsureClusterID(ctx)
	if err != nil {
		return startupResult{}, fmt.Errorf("load cluster ID: %w", err)
	}

	if reporter, ok := s.persistenceManager.(storeResetReporter); ok {
		s.stateReset = reporter.StoreWasReset()
	}
	isFirstRun, err := s.persistenceManager.IsFirstRun(ctx)
	if err != nil {
		return startupResult{}, fmt.Errorf("load startup state: %w", err)
	}

	s.currentVersion = version.Short()
	storedVersion, err := s.persistenceManager.GetStoredVersion(ctx)
	if err != nil {
		return startupResult{}, fmt.Errorf("load stored version: %w", err)
	}
	isUpgrade := storedVersion != "" && storedVersion != s.currentVersion

	// How long was nobody watching? kwatch runs as a single replica, so it
	// goes down with the cluster it is meant to report on — exactly when the
	// gap matters most. Saying so is the difference between "no alerts" and
	// "no alerts because nothing was looking".
	s.downtime, err = s.measureDowntime(ctx)
	if err != nil {
		return startupResult{}, fmt.Errorf("load monitoring gap: %w", err)
	}
	if err := s.loadRuntimeSession(ctx); err != nil {
		return startupResult{}, fmt.Errorf("load runtime session: %w", err)
	}

	if err := s.decideAnnouncement(ctx, isFirstRun, isUpgrade); err != nil {
		return startupResult{}, err
	}

	if err := s.persistenceManager.MarkAsInitialized(
		ctx,
		clusterID,
		s.currentVersion,
	); err != nil {
		return startupResult{}, fmt.Errorf("persist startup state: %w", err)
	}

	return startupResult{
		ClusterID: clusterID, CurrentVersion: s.currentVersion,
	}, nil
}

func (s *startupManager) loadRuntimeSession(ctx context.Context) error {
	store, ok := s.persistenceManager.(runtimeSessionStore)
	if !ok {
		return nil
	}
	previous, err := store.GetRuntimeSession(ctx)
	if err != nil {
		return err
	}
	s.restartReason = classifyRestart(previous)
	if s.evidenceSource != nil && previous.SessionID != "" &&
		previous.EndedAt.IsZero() {
		evidence, evidenceErr := s.evidenceSource.ReadRestartEvidence(
			ctx, previous,
		)
		if evidenceErr != nil {
			klog.V(2).InfoS(
				"restart evidence unavailable", "error", evidenceErr,
			)
		} else {
			s.restartReason = classifyWithEvidence(
				previous, evidence, s.restartReason,
			)
		}
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.session = runtimeSession{
		SessionID:     uuid.NewString(),
		PodName:       os.Getenv("POD_NAME"),
		PodUID:        os.Getenv("POD_UID"),
		NodeName:      os.Getenv("NODE_NAME"),
		StartedAt:     s.now(),
		LastHeartbeat: s.now(),
	}
	return store.SaveRuntimeSession(ctx, s.session)
}

func (s *startupManager) claimStartupAnnouncement(
	ctx context.Context,
	firstRun, upgrade bool,
) (bool, error) {
	store, ok := s.persistenceManager.(startupAnnouncementStore)
	if !ok {
		return true, nil
	}
	key := startupClaimKey(
		s.currentVersion, firstRun, upgrade, s.downtime, s.restartReason,
	)
	return store.ClaimStartupAnnouncement(ctx, key)
}

// runtimeSession records the last active process generation. An incomplete
// session is evidence of an unclean stop, but its reason is kept bounded so
// it is safe to expose in diagnostics. Its JSON shape is persisted.
type runtimeSession struct {
	SessionID     string    `json:"sessionID"`
	PodName       string    `json:"podName,omitempty"`
	PodUID        string    `json:"podUID,omitempty"`
	NodeName      string    `json:"nodeName,omitempty"`
	StartedAt     time.Time `json:"startedAt"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
	EndedAt       time.Time `json:"endedAt,omitempty"`
	EndReason     string    `json:"endReason,omitempty"`
}

// minReportableDowntime keeps ordinary restarts quiet. Rollouts and pod moves
// take a few minutes; only a gap longer than this says anything useful.
const minReportableDowntime = 5 * time.Minute

// measureDowntime compares the last recorded liveness stamp with now.
func (s *startupManager) measureDowntime(
	ctx context.Context,
) (time.Duration, error) {
	last, err := s.persistenceManager.GetLastSeen(ctx)
	if err != nil {
		return 0, err
	}
	if last.IsZero() {
		return 0, nil
	}
	gap := s.now().Sub(last)
	if gap < minReportableDowntime {
		return 0, nil
	}
	return gap, nil
}

// RecordAlive stamps the liveness marker used to measure the next gap.
func (s *startupManager) RecordAlive(ctx context.Context) {
	if err := s.persistenceManager.SetLastSeen(ctx, s.now()); err != nil {
		klog.V(2).InfoS("failed to record liveness stamp", "error", err)
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if store, ok := s.persistenceManager.(runtimeSessionStore); ok &&
		s.session.SessionID != "" {
		s.session.LastHeartbeat = s.now()
		if err := store.SaveRuntimeSession(ctx, s.session); err != nil {
			klog.V(2).InfoS("failed to record runtime session", "error", err)
		}
	}
}

// EndSession marks a controlled process stop. An unmarked session is used as
// evidence of an unexpected termination on the next active start.
func (s *startupManager) EndSession(ctx context.Context, reason string) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if s.session.SessionID == "" {
		return
	}
	store, ok := s.persistenceManager.(runtimeSessionStore)
	if !ok {
		return
	}
	s.session.EndedAt = s.now()
	s.session.EndReason = normalizeRestartReason(reason)
	if err := store.SaveRuntimeSession(ctx, s.session); err != nil {
		klog.V(2).InfoS("failed to close runtime session", "error", err)
	}
}
