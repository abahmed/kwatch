package incident

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/model"
)

// ConfigureAttributionSources installs the source port in one operation so
// partial lister wiring cannot be observed by attribution.
func (e *Engine) ConfigureAttributionSources(
	sources AttributionSources,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.processingStarted {
		return fmt.Errorf(
			"incident attribution sources cannot change after processing starts",
		)
	}
	if e.attributionConfigured {
		return fmt.Errorf("incident attribution sources are already configured")
	}
	if sources == nil {
		return fmt.Errorf("incident attribution sources are required")
	}
	e.attributionSources = sources
	e.attributionConfigured = true
	return nil
}

// Caller must hold e.mu.
func (e *Engine) indexIncidentByNamespace(inc *model.Incident) {
	ns, key := inc.Namespace, inc.Key
	if ns == "" {
		return
	}
	if e.namespaceIndex[ns] == nil {
		e.namespaceIndex[ns] = make(map[model.IncidentKey]*model.Incident)
	}
	e.namespaceIndex[ns][key] = inc
}

// Caller must hold e.mu.
func (e *Engine) removeIncidentFromNamespaceIndex(inc *model.Incident) {
	ns, key := inc.Namespace, inc.Key
	if ns == "" {
		return
	}
	delete(e.namespaceIndex[ns], key)
	if len(e.namespaceIndex[ns]) == 0 {
		delete(e.namespaceIndex, ns)
	}
}

// SetSubjectPresence installs the existence check used by stale cleanup. It
// is wired after informer caches exist, so it cannot be a constructor field.
func (e *Engine) SetSubjectPresence(
	fn func(resource, namespace, name string) (exists, known bool),
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.SubjectPresent = fn
}

func (e *Engine) SetSeverityMap(m map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if enricher, ok := e.config.Enricher.(*enricher.DefaultEnricher); ok {
		enricher.SetSeverityMap(m)
	}
}
