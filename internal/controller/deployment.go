package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
)

type multiDeploymentLister struct {
	listers []appsv1lister.DeploymentLister
}

func (m *multiDeploymentLister) List(selector labels.Selector) ([]*appsv1.Deployment, error) {
	var all []*appsv1.Deployment
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiDeploymentLister) Deployments(namespace string) appsv1lister.DeploymentNamespaceLister {
	nsl := make([]appsv1lister.DeploymentNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Deployments(namespace))
	}
	return &multiDeploymentNamespaceLister{listers: nsl}
}

type multiDeploymentNamespaceLister struct {
	listers []appsv1lister.DeploymentNamespaceLister
}

func (m *multiDeploymentNamespaceLister) List(selector labels.Selector) ([]*appsv1.Deployment, error) {
	var all []*appsv1.Deployment
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiDeploymentNamespaceLister) Get(name string) (*appsv1.Deployment, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, name)
}
