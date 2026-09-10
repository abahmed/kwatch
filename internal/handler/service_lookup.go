package handler

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ServiceLookup reports whether a Service exists. A non-NotFound error means
// the cache could not answer and must not be treated as a missing Service.
type ServiceLookup func(namespace, name string) (bool, error)

func (h *handler) serviceLookup() ServiceLookup {
	return func(namespace, name string) (bool, error) {
		if h.listers.Service == nil {
			return true, nil
		}
		_, err := h.listers.Service.Services(namespace).Get(name)
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
}
