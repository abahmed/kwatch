package startup

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/version"
)

// StateStore is the startup subset of persistence. Keeping this interface in
// the consumer package prevents startup from depending on the full storage
// manager and makes the lifecycle state flow explicit.
type StateStore interface {
	EnsureClusterID(context.Context) (string, error)
	IsFirstRun(context.Context) (bool, error)
	GetStoredVersion(context.Context) string
	MarkAsInitialized(context.Context, string, string) error
	GetLastSeen(context.Context) time.Time
	SetLastSeen(context.Context, time.Time) error
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
}

// NewStartupManagerWithRuntime constructs startup from the immutable runtime
// snapshot used by application composition.
func NewStartupManagerWithRuntime(
	state StateStore,
	runtime config.RuntimeConfig,
	now clock.Clock,
) *StartupManager {
	return newStartupManager(
		state, runtime.DisableStartupMessage(), now,
	)
}

func newStartupManager(
	state StateStore,
	disableStartupMessage bool,
	now clock.Clock,
) *StartupManager {
	if now == nil {
		now = clock.RealClock{}
	}
	sm := &StartupManager{
		persistenceManager:    state,
		disableStartupMessage: disableStartupMessage,
		now:                   now.Now,
	}
	return sm
}

// Start loads and records startup state, returning the decision needed by the
// application lifecycle. Nonessential state lookup failures remain nonfatal,
// preserving the historical startup behavior.
func (s *StartupManager) Start(ctx context.Context) (Result, error) {
	clusterID, err := s.persistenceManager.EnsureClusterID(ctx)
	if err != nil {
		klog.InfoS("failed to get/create cluster ID", "error", err)
		clusterID = ""
	}

	isFirstRun, err := s.persistenceManager.IsFirstRun(ctx)
	if err != nil {
		// Unknown state is not a first install. Avoid sending a misleading
		// welcome message when the API is temporarily unavailable or RBAC is
		// incomplete; initialization below will log its own failure as well.
		klog.InfoS("failed to determine whether this is the first run", "error", err)
		isFirstRun = false
	}

	s.currentVersion = version.Short()
	storedVersion := s.persistenceManager.GetStoredVersion(ctx)
	isUpgrade := storedVersion != "" && storedVersion != s.currentVersion

	// How long was nobody watching? kwatch runs as a single replica, so it
	// goes down with the cluster it is meant to report on — exactly when the
	// gap matters most. Saying so is the difference between "no alerts" and
	// "no alerts because nothing was looking".
	s.downtime = s.measureDowntime(ctx)

	s.shouldNotify = (isFirstRun || isUpgrade || s.downtime > 0) &&
		!s.disableStartupMessage

	if err := s.persistenceManager.MarkAsInitialized(
		ctx,
		clusterID,
		s.currentVersion,
	); err != nil {
		klog.InfoS("failed to mark as initialized", "error", err)
		return s.result(false, false, s.clusterID), nil
	}
	s.clusterID = clusterID

	return s.result(isFirstRun, isUpgrade, clusterID), nil
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
func (s *StartupManager) measureDowntime(ctx context.Context) time.Duration {
	last := s.persistenceManager.GetLastSeen(ctx)
	if last.IsZero() {
		return 0
	}
	gap := s.now().Sub(last)
	if gap < minReportableDowntime {
		return 0
	}
	return gap
}

// RecordAlive stamps the liveness marker used to measure the next gap.
func (s *StartupManager) RecordAlive(ctx context.Context) {
	if err := s.persistenceManager.SetLastSeen(ctx, s.now()); err != nil {
		klog.V(2).InfoS("failed to record liveness stamp", "error", err)
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
			"\n:warning: No monitoring between %s and %s UTC (%s) — anything "+
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
	return msg, true
}

// GetPersistenceManager returns the restart-safe persistence manager.
func (s *StartupManager) GetPersistenceManager() StateStore {
	return s.persistenceManager
}
