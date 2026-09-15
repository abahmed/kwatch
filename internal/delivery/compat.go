package delivery

import (
	"github.com/abahmed/kwatch/internal/config"
)

// Init is retained for package-local compatibility callers. New composition
// must use InitRuntime so delivery receives one compiled configuration view.
func (a *Manager) Init(
	alertCfg map[string]map[string]interface{},
	appCfg *config.App,
) {
	a.InitWithFactory(alertCfg, appCfg, nil)
}

// InitWithFactory is a compatibility seam for tests and older embedded
// callers. Application code must provide RuntimeConfig directly.
func (a *Manager) InitWithFactory(
	alertCfg map[string]map[string]interface{},
	appCfg *config.App,
	factory ProviderFactory,
) {
	runtime := config.RuntimeConfigFor(&config.Config{Alert: alertCfg})
	if appCfg == nil {
		appCfg = &config.App{}
	}
	// Compatibility callers retain the historical application identity. New
	// production code uses InitRuntime and derives it from RuntimeConfig.
	runtime = withApplicationIdentity(runtime, appCfg.ClusterName)
	a.initRuntime(runtime, factory)
}

func withApplicationIdentity(
	runtime config.RuntimeConfig,
	clusterName string,
) config.RuntimeConfig {
	return config.WithApplicationIdentity(runtime, clusterName)
}
