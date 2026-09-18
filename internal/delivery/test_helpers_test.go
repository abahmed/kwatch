package delivery

import (
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func newTestManager() *Manager {
	return &Manager{
		providerDeps: transport.Dependencies{Clock: clock.RealClock{}},
		now:          clock.RealClock{}.Now,
	}
}

func managerWithEntries(entries []providerEntry) *Manager {
	manager := newTestManager()
	manager.generation = newProviderGeneration(entries)
	return manager
}

func setManagerEntries(manager *Manager, entries []providerEntry) {
	manager.generation = newProviderGeneration(entries)
	if manager.now == nil {
		manager.now = clock.RealClock{}.Now
	}
}

func appendManagerEntries(manager *Manager, entries ...providerEntry) {
	current := generationEntries(manager.generation)
	setManagerEntries(manager, append(current, entries...))
}

func managerEntries(manager *Manager) []providerEntry {
	return generationEntries(manager.generation)
}

func initTestManager(
	manager *Manager,
	alertSettings map[string]map[string]interface{},
	appConfig *config.App,
	factory ProviderFactory,
) error {
	cfg := &config.Config{Alert: alertSettings}
	if appConfig != nil {
		cfg.App = *appConfig
	}
	return manager.InitRuntime(config.RuntimeConfigFor(cfg), factory)
}

func setTestSilences(manager *Manager, rules []config.SilenceRule) {
	manager.cfgMu.Lock()
	defer manager.cfgMu.Unlock()
	manager.silences = compileSilences(rules)
}

func setTestTemplates(manager *Manager, templates map[string]string) {
	manager.cfgMu.Lock()
	defer manager.cfgMu.Unlock()
	manager.templates = compileTemplates(templates)
}
