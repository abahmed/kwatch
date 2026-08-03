package controller

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiServiceLister struct {
	listers []corev1lister.ServiceLister
}

func (m *multiServiceLister) List(selector labels.Selector) ([]*corev1.Service, error) {
	var all []*corev1.Service
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiServiceLister) Services(namespace string) corev1lister.ServiceNamespaceLister {
	nsl := make([]corev1lister.ServiceNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Services(namespace))
	}
	return &multiServiceNamespaceLister{listers: nsl}
}

type multiServiceNamespaceLister struct {
	listers []corev1lister.ServiceNamespaceLister
}

func (m *multiServiceNamespaceLister) List(selector labels.Selector) ([]*corev1.Service, error) {
	var all []*corev1.Service
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiServiceNamespaceLister) Get(name string) (*corev1.Service, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, name)
}
