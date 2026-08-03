package controller

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiConfigMapLister struct {
	listers []corev1lister.ConfigMapLister
}

func (m *multiConfigMapLister) List(selector labels.Selector) ([]*corev1.ConfigMap, error) {
	var all []*corev1.ConfigMap
	for _, l := range m.listers {
		cms, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, cms...)
	}
	return all, nil
}

func (m *multiConfigMapLister) ConfigMaps(namespace string) corev1lister.ConfigMapNamespaceLister {
	nsl := make([]corev1lister.ConfigMapNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.ConfigMaps(namespace))
	}
	return &multiConfigMapNamespaceLister{listers: nsl}
}

type multiConfigMapNamespaceLister struct {
	listers []corev1lister.ConfigMapNamespaceLister
}

func (m *multiConfigMapNamespaceLister) List(selector labels.Selector) ([]*corev1.ConfigMap, error) {
	var all []*corev1.ConfigMap
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiConfigMapNamespaceLister) Get(name string) (*corev1.ConfigMap, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, name)
}
