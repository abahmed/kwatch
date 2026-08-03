package controller

import (
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
)

type multiIngressLister struct {
	listers []networkingv1lister.IngressLister
}

func (m *multiIngressLister) List(selector labels.Selector) ([]*networkingv1.Ingress, error) {
	var all []*networkingv1.Ingress
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiIngressLister) Ingresses(namespace string) networkingv1lister.IngressNamespaceLister {
	nsl := make([]networkingv1lister.IngressNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.Ingresses(namespace))
	}
	return &multiIngressNamespaceLister{listers: nsl}
}

type multiIngressNamespaceLister struct {
	listers []networkingv1lister.IngressNamespaceLister
}

func (m *multiIngressNamespaceLister) List(selector labels.Selector) ([]*networkingv1.Ingress, error) {
	var all []*networkingv1.Ingress
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiIngressNamespaceLister) Get(name string) (*networkingv1.Ingress, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "networking.k8s.io", Resource: "ingresses"}, name)
}
