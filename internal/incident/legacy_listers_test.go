package incident

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

// legacyListerSources keeps old package-local tests focused on behavior while
// production code uses the single AttributionSources port.
type legacyListerSources struct {
	deployments appsv1lister.DeploymentLister
	statefulSet appsv1lister.StatefulSetLister
	daemonSet   appsv1lister.DaemonSetLister
	services    corev1lister.ServiceLister
}

func (s *legacyListerSources) Deployment(
	namespace, name string,
) (*appsv1.Deployment, error) {
	if s.deployments == nil {
		return nil, nil
	}
	return s.deployments.Deployments(namespace).Get(name)
}

func (s *legacyListerSources) StatefulSet(
	namespace, name string,
) (*appsv1.StatefulSet, error) {
	if s.statefulSet == nil {
		return nil, nil
	}
	return s.statefulSet.StatefulSets(namespace).Get(name)
}

func (s *legacyListerSources) DaemonSet(
	namespace, name string,
) (*appsv1.DaemonSet, error) {
	if s.daemonSet == nil {
		return nil, nil
	}
	return s.daemonSet.DaemonSets(namespace).Get(name)
}

func (s *legacyListerSources) Service(
	namespace, name string,
) (*corev1.Service, error) {
	if s.services == nil {
		return nil, nil
	}
	return s.services.Services(namespace).Get(name)
}

func (s *legacyListerSources) ListServices(
	namespace string,
) ([]*corev1.Service, error) {
	if s.services == nil {
		return nil, nil
	}
	return s.services.Services(namespace).List(labels.Everything())
}

func (e *Engine) legacySources() *legacyListerSources {
	if sources, ok := e.attributionSources.(*legacyListerSources); ok {
		return sources
	}
	sources := &legacyListerSources{}
	e.attributionSources = sources
	return sources
}

func setTestDeployLister(
	e *Engine,
	l appsv1lister.DeploymentLister,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.legacySources().deployments = l
}

func setTestStatefulSetLister(
	e *Engine,
	l appsv1lister.StatefulSetLister,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.legacySources().statefulSet = l
}

func setTestDaemonSetLister(
	e *Engine,
	l appsv1lister.DaemonSetLister,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.legacySources().daemonSet = l
}

func setTestServiceLister(
	e *Engine,
	l corev1lister.ServiceLister,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.legacySources().services = l
}
