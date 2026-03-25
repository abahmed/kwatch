package startup

import (
	"context"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/abahmed/kwatch/internal/telemetry"
	"github.com/abahmed/kwatch/internal/version"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type StartupManager struct {
	stateManager *state.StateManager
	telemetry    *telemetry.Telemetry
	alertManager *alertmanager.AlertManager
	config       *config.Config
}

func NewStartupManager(
	client kubernetes.Interface,
	namespace string,
	telemetryCfg *config.Telemetry,
	alertCfg map[string]map[string]interface{},
	appCfg *config.App,
) *StartupManager {
	sm := &StartupManager{
		stateManager: state.NewStateManager(client, namespace),
		telemetry:    telemetry.NewTelemetry(telemetryCfg),
		config:       &config.Config{App: *appCfg},
	}

	sm.alertManager = &alertmanager.AlertManager{}
	sm.alertManager.Init(alertCfg, appCfg)

	return sm
}

func (s *StartupManager) HandleStartup(ctx context.Context) error {
	clusterID, err := s.stateManager.EnsureClusterID(ctx)
	if err != nil {
		logrus.Warnf("failed to get/create cluster ID: %v", err)
		clusterID = ""
	}

	isFirstRun, _ := s.stateManager.IsFirstRun(ctx)

	currentVersion := version.Short()
	storedVersion := s.stateManager.GetStoredVersion(ctx)
	isUpgrade := storedVersion != "" && storedVersion != currentVersion

	sendNotification := (isFirstRun || isUpgrade) && !s.config.App.DisableStartupMessage
	sendTelemetry := s.telemetry != nil && (isFirstRun || isUpgrade) &&
		!s.stateManager.IsTelemetrySent(ctx)

	if sendNotification {
		s.alertManager.Notify(
			constant.WelcomeMsg)
	}

	if sendTelemetry {
		if err := s.telemetry.SendEvent(ctx, clusterID, currentVersion); err != nil {
			logrus.Warnf("failed to send telemetry event: %v", err)
		} else {
			if err := s.stateManager.MarkTelemetrySent(ctx); err != nil {
				logrus.Warnf("failed to mark telemetry as sent: %v", err)
			}
		}
	}

	if err := s.stateManager.MarkAsInitialized(ctx, clusterID, currentVersion); err != nil {
		logrus.Warnf("failed to mark as initialized: %v", err)
	}

	return nil
}

func (s *StartupManager) GetAlertManager() *alertmanager.AlertManager {
	return s.alertManager
}

func (s *StartupManager) GetStateManager() *state.StateManager {
	return s.stateManager
}
