package controller

import (
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
)

type multiNetpolLister struct {
	listers []networkingv1lister.NetworkPolicyLister
}

func (m *multiNetpolLister) List(selector labels.Selector) ([]*networkingv1.NetworkPolicy, error) {
	var all []*networkingv1.NetworkPolicy
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiNetpolLister) NetworkPolicies(namespace string) networkingv1lister.NetworkPolicyNamespaceLister {
	nsl := make([]networkingv1lister.NetworkPolicyNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.NetworkPolicies(namespace))
	}
	return &multiNetpolNamespaceLister{listers: nsl}
}

type multiNetpolNamespaceLister struct {
	listers []networkingv1lister.NetworkPolicyNamespaceLister
}

func (m *multiNetpolNamespaceLister) List(selector labels.Selector) ([]*networkingv1.NetworkPolicy, error) {
	var all []*networkingv1.NetworkPolicy
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiNetpolNamespaceLister) Get(name string) (*networkingv1.NetworkPolicy, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"}, name)
}
