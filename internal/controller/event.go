package controller

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiEventLister struct {
	listers []corev1lister.EventLister
}

func (m *multiEventLister) List(selector labels.Selector) ([]*corev1.Event, error) {
	var all []*corev1.Event
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiEventLister) Events(namespace string) corev1lister.EventNamespaceLister {
	nsl := make([]corev1lister.EventNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Events(namespace))
	}
	return &multiEventNamespaceLister{listers: nsl}
}

type multiEventNamespaceLister struct {
	listers []corev1lister.EventNamespaceLister
}

func (m *multiEventNamespaceLister) List(selector labels.Selector) ([]*corev1.Event, error) {
	var all []*corev1.Event
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiEventNamespaceLister) Get(name string) (*corev1.Event, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "events"}, name)
}
