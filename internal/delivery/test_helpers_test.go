package delivery

import "github.com/abahmed/kwatch/internal/config"

func managerWithEntries(entries []providerEntry) *Manager {
	return &Manager{generation: newProviderGeneration(entries)}
}

func setManagerEntries(manager *Manager, entries []providerEntry) {
	manager.generation = newProviderGeneration(entries)
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
) {
	cfg := &config.Config{Alert: alertSettings}
	if appConfig != nil {
		cfg.App = *appConfig
	}
	manager.InitRuntime(config.RuntimeConfigFor(cfg), factory)
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
