package startup

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/version"
)

// StateStore is the startup subset of persistence. Keeping this interface in
// the consumer package prevents startup from depending on the full storage
// manager and makes the lifecycle state flow explicit.
type StateStore interface {
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
	GetRuntimeSession(context.Context) (model.RuntimeSession, error)
	SaveRuntimeSession(context.Context, model.RuntimeSession) error
}

// RestartEvidence is the bounded, application-owned evidence used to explain
// an incomplete monitoring session. Startup does not depend on client-go;
// the application supplies this value through EvidenceSource.
type RestartEvidence struct {
	PodReason       string
	ContainerReason string
	NodeObserved    bool
	NodeReady       bool
	NodeReason      string
	LeaseLost       bool
	APIUnavailable  bool
}

// EvidenceSource adapts Kubernetes state to the startup classification
// contract. Implementations must return bounded fields and may report no
// evidence when the previous object has already been removed.
type EvidenceSource interface {
	ReadRestartEvidence(
		context.Context, model.RuntimeSession,
	) (RestartEvidence, error)
}

type StartupManager struct {
	persistenceManager    StateStore
	disableStartupMessage bool
	shouldNotify          bool
	// downtime is how long monitoring was unavailable before this start,
	// zero when there is no previous record or the gap was insignificant.
	downtime       time.Duration
	currentVersion string
	clusterID      string
	session        model.RuntimeSession
	restartReason  string
	evidenceSource EvidenceSource
	now            func() time.Time
}

// Result is the immutable startup decision consumed by application
// composition. Keeping it explicit makes first-run, upgrade, and downtime
// state visible without making delivery part of startup persistence.
type Result struct {
	ClusterID      string
	CurrentVersion string
	FirstRun       bool
	Upgrade        bool
	Downtime       time.Duration
	ShouldNotify   bool
	RestartReason  string
}

// NewStartupManagerWithRuntime constructs startup from the immutable runtime
// snapshot used by application composition.
func NewStartupManagerWithRuntime(
	state StateStore,
	runtime config.RuntimeConfig,
	now clock.Clock,
	evidence ...EvidenceSource,
) *StartupManager {
	manager := newStartupManager(
		state, runtime.Application().DisableStartupMessage, now,
	)
	if len(evidence) > 0 {
		manager.evidenceSource = evidence[0]
	}
	return manager
}

func newStartupManager(
	state StateStore,
	disableStartupMessage bool,
	now clock.Clock,
) *StartupManager {
	now = clock.Require(now)
	sm := &StartupManager{
		persistenceManager:    state,
		disableStartupMessage: disableStartupMessage,
		now:                   now.Now,
	}
	return sm
}

// Start loads and records startup state, returning the decision needed by the
// application lifecycle. State needed to fence a new active generation is
// fail-closed: an API or RBAC error must not look like a first run.
func (s *StartupManager) Start(ctx context.Context) (Result, error) {
	clusterID, err := s.persistenceManager.EnsureClusterID(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load cluster ID: %w", err)
	}

	isFirstRun, err := s.persistenceManager.IsFirstRun(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load startup state: %w", err)
	}

	s.currentVersion = version.Short()
	storedVersion, err := s.persistenceManager.GetStoredVersion(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load stored version: %w", err)
	}
	isUpgrade := storedVersion != "" && storedVersion != s.currentVersion

	// How long was nobody watching? kwatch runs as a single replica, so it
	// goes down with the cluster it is meant to report on — exactly when the
	// gap matters most. Saying so is the difference between "no alerts" and
	// "no alerts because nothing was looking".
	s.downtime, err = s.measureDowntime(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load monitoring gap: %w", err)
	}
	if err := s.loadRuntimeSession(ctx); err != nil {
		return Result{}, fmt.Errorf("load runtime session: %w", err)
	}

	s.shouldNotify = (isFirstRun || isUpgrade || s.downtime > 0) &&
		!s.disableStartupMessage
	if s.restartReason == "internal_failure" &&
		!s.disableStartupMessage {
		s.shouldNotify = true
	}
	if s.shouldNotify {
		claimed, claimErr := s.claimStartupAnnouncement(
			ctx, isFirstRun, isUpgrade,
		)
		if claimErr != nil {
			return Result{}, fmt.Errorf(
				"claim startup announcement: %w", claimErr,
			)
		}
		s.shouldNotify = claimed
	}

	if err := s.persistenceManager.MarkAsInitialized(
		ctx,
		clusterID,
		s.currentVersion,
	); err != nil {
		return Result{}, fmt.Errorf("persist startup state: %w", err)
	}
	s.clusterID = clusterID

	return s.result(isFirstRun, isUpgrade, clusterID), nil
}

func (s *StartupManager) loadRuntimeSession(ctx context.Context) error {
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
	s.session = model.RuntimeSession{
		SessionID:     uuid.NewString(),
		PodName:       os.Getenv("POD_NAME"),
		PodUID:        os.Getenv("POD_UID"),
		NodeName:      os.Getenv("NODE_NAME"),
		StartedAt:     s.now(),
		LastHeartbeat: s.now(),
	}
	return store.SaveRuntimeSession(ctx, s.session)
}

func (s *StartupManager) claimStartupAnnouncement(
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

// HandleStartup is retained for callers that only need the historical error
// contract. New composition code should use Start and its typed Result.
func (s *StartupManager) HandleStartup(ctx context.Context) error {
	_, err := s.Start(ctx)
	return err
}

func (s *StartupManager) result(
	firstRun, upgrade bool,
	clusterID string,
) Result {
	return Result{
		ClusterID:      clusterID,
		CurrentVersion: s.currentVersion,
		FirstRun:       firstRun,
		Upgrade:        upgrade,
		Downtime:       s.downtime,
		ShouldNotify:   s.shouldNotify,
		RestartReason:  s.restartReason,
	}
}

// TelemetryIdentity returns the stable cluster identity and current version
// after startup state has been persisted successfully.
func (s *StartupManager) TelemetryIdentity() (string, string) {
	return s.clusterID, s.currentVersion
}

// minReportableDowntime keeps ordinary restarts quiet. Rollouts and pod moves
// take a few minutes; only a gap longer than this says anything useful.
const minReportableDowntime = 5 * time.Minute

// measureDowntime compares the last recorded liveness stamp with now.
func (s *StartupManager) measureDowntime(
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
func (s *StartupManager) RecordAlive(ctx context.Context) {
	if err := s.persistenceManager.SetLastSeen(ctx, s.now()); err != nil {
		klog.V(2).InfoS("failed to record liveness stamp", "error", err)
	}
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
func (s *StartupManager) EndSession(ctx context.Context, reason string) {
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

// RecordFailure preserves bounded failure evidence before the final session
// marker is written. The raw error remains in logs only; persisted state must
// be safe to expose through diagnostics.
func (s *StartupManager) RecordFailure(
	ctx context.Context, component, code string,
) {
	if s.session.SessionID == "" {
		return
	}
	store, ok := s.persistenceManager.(runtimeSessionStore)
	if !ok {
		return
	}
	s.session.FailedComponent = boundedFailureField(component)
	s.session.FailureCode = normalizeFailureCode(code)
	if err := store.SaveRuntimeSession(ctx, s.session); err != nil {
		klog.V(2).InfoS("failed to persist runtime failure", "error", err)
	}
}

func boundedFailureField(value string) string {
	if len(value) > 64 {
		return value[:64]
	}
	return value
}

func normalizeFailureCode(code string) string {
	switch code {
	case "api_unavailable", "leader_handoff", "internal_failure":
		return code
	default:
		return "internal_failure"
	}
}

func classifyRestart(previous model.RuntimeSession) string {
	if previous.SessionID == "" {
		return ""
	}
	if previous.FailureCode != "" {
		return normalizeRestartReason(previous.FailureCode)
	}
	if previous.EndReason != "" {
		return normalizeRestartReason(previous.EndReason)
	}
	if !previous.EndedAt.IsZero() {
		return ""
	}
	if previous.NodeName != "" && previous.NodeName != os.Getenv("NODE_NAME") {
		return "node_disruption"
	}
	if previous.PodName != "" && previous.PodName != os.Getenv("POD_NAME") {
		return "deployment_rollout"
	}
	return "internal_failure"
}

func classifyWithEvidence(
	previous model.RuntimeSession,
	evidence RestartEvidence,
	fallback string,
) string {
	if previous.FailureCode != "" || previous.EndReason != "" {
		return fallback
	}
	if evidence.APIUnavailable {
		return "api_unavailable"
	}
	if evidence.ContainerReason == "OOMKilled" ||
		evidence.PodReason == "OOMKilled" {
		return "oom_killed"
	}
	if evidence.PodReason == "Evicted" {
		return "eviction"
	}
	if evidence.LeaseLost {
		return "leader_handoff"
	}
	if !evidence.NodeReady &&
		(evidence.NodeObserved || evidence.NodeReason != "") {
		return "node_disruption"
	}
	return fallback
}

func startupClaimKey(
	version string,
	firstRun, upgrade bool,
	downtime time.Duration,
	restartReason string,
) string {
	return fmt.Sprintf(
		"%s|first=%t|upgrade=%t|downtime=%d|reason=%s",
		version, firstRun, upgrade,
		downtime.Round(time.Minute).Nanoseconds(), restartReason,
	)
}

func normalizeRestartReason(reason string) string {
	switch reason {
	case "graceful_shutdown", "deployment_rollout", "leader_handoff",
		"node_disruption", "eviction", "oom_killed", "internal_failure",
		"api_unavailable", "unknown":
		return reason
	default:
		return "unknown"
	}
}

// StartupMessage returns the one-time startup message when the application
// should notify operators. Startup owns the state decision; delivery owns the
// transport and is intentionally outside this package.
func (s *StartupManager) StartupMessage() (string, bool) {
	if !s.shouldNotify {
		return "", false
	}
	msg := fmt.Sprintf(constant.WelcomeMsg, s.currentVersion)
	if s.downtime > 0 {
		gapEnd := s.now()
		gapStart := gapEnd.Add(-s.downtime)
		msg += fmt.Sprintf(
			"\n⚠️ No monitoring between %s and %s UTC (%s) — anything "+
				"that broke in that window went unreported.",
			gapStart.UTC().Format("15:04"),
			gapEnd.UTC().Format("15:04"),
			format.Duration(s.downtime.Round(time.Minute)),
		)
		klog.InfoS(
			"monitoring gap detected",
			"downtime",
			s.downtime.Round(time.Minute),
		)
	}
	if s.restartReason != "" {
		msg += fmt.Sprintf("\n⚠️ Monitoring resumed after %s.",
			humanRestartReason(s.restartReason))
	}
	return msg, true
}

func humanRestartReason(reason string) string {
	switch reason {
	case "node_disruption":
		return "a node disruption"
	case "deployment_rollout":
		return "a Kwatch rollout"
	case "leader_handoff":
		return "a leader handoff"
	case "eviction":
		return "an eviction"
	case "oom_killed":
		return "an out-of-memory termination"
	case "api_unavailable":
		return "a Kubernetes API outage"
	case "internal_failure":
		return "an unexpected Kwatch stop"
	default:
		return "an interrupted monitoring session"
	}
}

// GetPersistenceManager returns the restart-safe persistence manager.
func (s *StartupManager) GetPersistenceManager() StateStore {
	return s.persistenceManager
}
