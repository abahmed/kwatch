package startup

import (
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

// NewStartupManagerWithClock is retained for embedded callers that still
// provide YAML-facing application settings. Production composition should use
// NewStartupManagerWithRuntime.
func NewStartupManagerWithClock(
	state StateStore,
	appCfg config.App,
	now clock.Clock,
) *StartupManager {
	return newStartupManager(state, appCfg.DisableStartupMessage, now)
}

// NewStartupManager is retained for embedded callers that still provide the
// YAML-facing application settings. Production composition should use
// NewStartupManagerWithRuntime.
func NewStartupManager(
	state StateStore,
	appCfg *config.App,
) *StartupManager {
	if appCfg == nil {
		appCfg = &config.App{}
	}
	return newStartupManager(
		state, appCfg.DisableStartupMessage, clock.RealClock{},
	)
}
