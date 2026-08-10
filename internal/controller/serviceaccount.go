package controller

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiServiceAccountLister struct {
	listers []corev1lister.ServiceAccountLister
}

func (m *multiServiceAccountLister) List(selector labels.Selector) ([]*corev1.ServiceAccount, error) {
	var all []*corev1.ServiceAccount
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiServiceAccountLister) ServiceAccounts(namespace string) corev1lister.ServiceAccountNamespaceLister {
	nsl := make([]corev1lister.ServiceAccountNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.ServiceAccounts(namespace))
	}
	return &multiServiceAccountNamespaceLister{listers: nsl}
}

type multiServiceAccountNamespaceLister struct {
	listers []corev1lister.ServiceAccountNamespaceLister
}

func (m *multiServiceAccountNamespaceLister) List(selector labels.Selector) ([]*corev1.ServiceAccount, error) {
	var all []*corev1.ServiceAccount
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiServiceAccountNamespaceLister) Get(name string) (*corev1.ServiceAccount, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "serviceaccounts"}, name)
}
