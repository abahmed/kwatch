package controller

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type multiSecretLister struct {
	listers []corev1lister.SecretLister
}

func (m *multiSecretLister) List(selector labels.Selector) ([]*corev1.Secret, error) {
	var all []*corev1.Secret
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiSecretLister) Secrets(namespace string) corev1lister.SecretNamespaceLister {
	nsl := make([]corev1lister.SecretNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Secrets(namespace))
	}
	return &multiSecretNamespaceLister{listers: nsl}
}

type multiSecretNamespaceLister struct {
	listers []corev1lister.SecretNamespaceLister
}

func (m *multiSecretNamespaceLister) List(selector labels.Selector) ([]*corev1.Secret, error) {
	var all []*corev1.Secret
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiSecretNamespaceLister) Get(name string) (*corev1.Secret, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, name)
}
