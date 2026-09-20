package incident

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// AttributionSources is the incident-owned port for workload and service
// health lookups. Kubernetes listers are adapted at the composition boundary.
type AttributionSources interface {
	Deployment(namespace, name string) (*appsv1.Deployment, error)
	StatefulSet(namespace, name string) (*appsv1.StatefulSet, error)
	DaemonSet(namespace, name string) (*appsv1.DaemonSet, error)
	Service(namespace, name string) (*corev1.Service, error)
	ListServices(namespace string) ([]*corev1.Service, error)
}
