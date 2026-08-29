package startup

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/abahmed/kwatch/internal/version"
)

type StartupManager struct {
	stateManager *state.StateManager
	alertManager *alert.AlertManager
	config       *config.Config
	shouldNotify bool
	// downtime is how long monitoring was unavailable before this start,
	// zero when there is no previous record or the gap was insignificant.
	downtime       time.Duration
	currentVersion string
	now            func() time.Time
}

func NewStartupManager(
	client kubernetes.Interface,
	namespace string,
	alertCfg map[string]map[string]interface{},
	appCfg *config.App,
) *StartupManager {
	sm := &StartupManager{
		stateManager: state.NewStateManager(client, namespace),
		config:       &config.Config{App: *appCfg},
		now:          time.Now,
	}

	sm.alertManager = &alert.AlertManager{}
	sm.alertManager.Init(alertCfg, appCfg)

	return sm
}

func (s *StartupManager) HandleStartup(ctx context.Context) error {
	clusterID, err := s.stateManager.EnsureClusterID(ctx)
	if err != nil {
		klog.InfoS("failed to get/create cluster ID", "error", err)
		clusterID = ""
	}

	isFirstRun, err := s.stateManager.IsFirstRun(ctx)
	if err != nil {
		// Unknown state is not a first install. Avoid sending a misleading
		// welcome message when the API is temporarily unavailable or RBAC is
		// incomplete; initialization below will log its own failure as well.
		klog.InfoS("failed to determine whether this is the first run", "error", err)
		isFirstRun = false
	}

	s.currentVersion = version.Short()
	storedVersion := s.stateManager.GetStoredVersion(ctx)
	isUpgrade := storedVersion != "" && storedVersion != s.currentVersion

	// How long was nobody watching? kwatch runs as a single replica, so it
	// goes down with the cluster it is meant to report on — exactly when the
	// gap matters most. Saying so is the difference between "no alerts" and
	// "no alerts because nothing was looking".
	s.downtime = s.measureDowntime(ctx)

	s.shouldNotify = (isFirstRun || isUpgrade || s.downtime > 0) &&
		!s.config.App.DisableStartupMessage

	if err := s.stateManager.MarkAsInitialized(
		ctx,
		clusterID,
		s.currentVersion,
	); err != nil {
		klog.InfoS("failed to mark as initialized", "error", err)
	}

	return nil
}

// minReportableDowntime keeps ordinary restarts quiet. Rollouts and pod moves
// take a few minutes; only a gap longer than this says anything useful.
const minReportableDowntime = 5 * time.Minute

// measureDowntime compares the last recorded liveness stamp with now.
func (s *StartupManager) measureDowntime(ctx context.Context) time.Duration {
	last := s.stateManager.GetLastSeen(ctx)
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
	if err := s.stateManager.SetLastSeen(ctx, s.now()); err != nil {
		klog.V(2).InfoS("failed to record liveness stamp", "error", err)
	}
}

func (s *StartupManager) NotifyStartup() {
	if !s.shouldNotify {
		return
	}
	msg := fmt.Sprintf(constant.WelcomeMsg, s.currentVersion)
	if s.downtime > 0 {
		msg += fmt.Sprintf(
			"\n:warning: No monitoring for %s before this start — anything "+
				"that broke in that window went unreported.",
			s.downtime.Round(time.Minute),
		)
		klog.InfoS(
			"monitoring gap detected",
			"downtime",
			s.downtime.Round(time.Minute),
		)
	}
	s.alertManager.Notify(msg)
}

func (s *StartupManager) GetAlertManager() *alert.AlertManager {
	return s.alertManager
}

func (s *StartupManager) GetStateManager() *state.StateManager {
	return s.stateManager
}
