package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/incident"
)

// attributionSources adapts synchronized controller listers to the small
// source port consumed by incident attribution.
type attributionSources struct {
	deployments appsv1lister.DeploymentLister
	statefulSet appsv1lister.StatefulSetLister
	daemonSet   appsv1lister.DaemonSetLister
	services    corev1lister.ServiceLister
}

func (s attributionSources) Deployment(
	namespace, name string,
) (*appsv1.Deployment, error) {
	if s.deployments == nil {
		return nil, nil
	}
	return s.deployments.Deployments(namespace).Get(name)
}

func (s attributionSources) StatefulSet(
	namespace, name string,
) (*appsv1.StatefulSet, error) {
	if s.statefulSet == nil {
		return nil, nil
	}
	return s.statefulSet.StatefulSets(namespace).Get(name)
}

func (s attributionSources) DaemonSet(
	namespace, name string,
) (*appsv1.DaemonSet, error) {
	if s.daemonSet == nil {
		return nil, nil
	}
	return s.daemonSet.DaemonSets(namespace).Get(name)
}

func (s attributionSources) Service(
	namespace, name string,
) (*corev1.Service, error) {
	if s.services == nil {
		return nil, nil
	}
	return s.services.Services(namespace).Get(name)
}

func (s attributionSources) ListServices(
	namespace string,
) ([]*corev1.Service, error) {
	if s.services == nil {
		return nil, nil
	}
	return s.services.Services(namespace).List(labels.Everything())
}

func (c *Controller) IncidentAttributionSources() incident.AttributionSources {
	return attributionSources{
		deployments: c.deployLister,
		statefulSet: c.ssLister,
		daemonSet:   c.dsLister,
		services:    c.serviceLister,
	}
}
