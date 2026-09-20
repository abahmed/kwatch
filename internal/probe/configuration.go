package probe

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// servicesFor reads Services from the informer cache, or nil when the cache
// is unavailable so the caller falls back to the API server.
func (m *Monitor) servicesFor(namespace string) []*corev1.Service {
	m.mu.Lock()
	lister := m.serviceLister
	m.mu.Unlock()
	if lister == nil {
		return nil
	}
	var (
		services []*corev1.Service
		err      error
	)
	if namespace == "" {
		services, err = lister.List(labels.Everything())
	} else {
		services, err = lister.Services(namespace).List(labels.Everything())
	}
	if err != nil {
		return nil
	}
	return services
}
