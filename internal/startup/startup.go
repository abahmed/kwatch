package startup

import (
	"context"
	"fmt"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/abahmed/kwatch/internal/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type StartupManager struct {
	stateManager *state.StateManager
	alertManager *alert.AlertManager
	config       *config.Config
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

	isFirstRun, _ := s.stateManager.IsFirstRun(ctx)

	currentVersion := version.Short()
	storedVersion := s.stateManager.GetStoredVersion(ctx)
	isUpgrade := storedVersion != "" && storedVersion != currentVersion

	sendNotification := (isFirstRun || isUpgrade) && !s.config.App.DisableStartupMessage

	if sendNotification {
		s.alertManager.Notify(
			fmt.Sprintf(constant.WelcomeMsg, currentVersion))
	}

	if err := s.stateManager.MarkAsInitialized(ctx, clusterID, currentVersion); err != nil {
		klog.InfoS("failed to mark as initialized", "error", err)
	}

	return nil
}

func (s *StartupManager) GetAlertManager() *alert.AlertManager {
	return s.alertManager
}

func (s *StartupManager) GetStateManager() *state.StateManager {
	return s.stateManager
}
