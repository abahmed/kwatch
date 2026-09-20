package pvc

import "fmt"

// Sources are the controller-owned namespace inputs used by PVC monitoring.
type Sources struct {
	Allowed          []string
	Forbidden        []string
	WatchAll         bool
	NamespaceAllowed func(string) bool
}

// ConfigureSources freezes namespace scope before the monitor starts.
func (p *PvcMonitor) ConfigureSources(sources Sources) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return fmt.Errorf("pvc sources cannot change after processing starts")
	}
	if p.configured {
		return fmt.Errorf("pvc sources are already configured")
	}
	p.configured = true
	p.setNamespaceScopeLocked(
		sources.Allowed, sources.Forbidden, sources.WatchAll,
	)
	p.namespaceFilter = sources.NamespaceAllowed
	return nil
}
