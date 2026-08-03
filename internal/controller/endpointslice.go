package controller

import (
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
)

type multiEndpointSliceLister struct {
	listers []discoveryv1lister.EndpointSliceLister
}

func (m *multiEndpointSliceLister) List(selector labels.Selector) ([]*discoveryv1.EndpointSlice, error) {
	var all []*discoveryv1.EndpointSlice
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiEndpointSliceLister) EndpointSlices(namespace string) discoveryv1lister.EndpointSliceNamespaceLister {
	nsl := make([]discoveryv1lister.EndpointSliceNamespaceLister, 0, len(m.listers))
	for _, l := range m.listers {
		nsl = append(nsl, l.EndpointSlices(namespace))
	}
	return &multiEndpointSliceNamespaceLister{listers: nsl}
}

type multiEndpointSliceNamespaceLister struct {
	listers []discoveryv1lister.EndpointSliceNamespaceLister
}

func (m *multiEndpointSliceNamespaceLister) List(selector labels.Selector) ([]*discoveryv1.EndpointSlice, error) {
	var all []*discoveryv1.EndpointSlice
	for _, l := range m.listers {
		items, err := l.List(selector)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (m *multiEndpointSliceNamespaceLister) Get(name string) (*discoveryv1.EndpointSlice, error) {
	for _, l := range m.listers {
		item, err := l.Get(name)
		if err == nil {
			return item, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "endpointslices"}, name)
}
