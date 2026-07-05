package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

func DetectServiceEndpointIssue(svc *corev1.Service, eps *corev1.Endpoints) *event.Signal {
	if svc.Spec.Selector == nil || len(svc.Spec.Selector) == 0 {
		return nil
	}
	if svc.Spec.ClusterIP == "None" || svc.Spec.ClusterIP == "" {
		return nil
	}
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}

	hasReady := false
	for _, subset := range eps.Subsets {
		if len(subset.Addresses) > 0 {
			hasReady = true
			break
		}
	}

	if !hasReady {
		key := svc.Namespace + "/" + svc.Name
		return &event.Signal{
			Resource:  "service",
			Namespace: svc.Namespace,
			Reason:    "ServiceNoEndpoints",
			Owner:     key,
			PodName:   svc.Name,
			Labels:    svc.Labels,
			Hint:      fmt.Sprintf("service %s has selectors but no ready endpoints", key),
		}
	}
	return nil
}

func (h *handler) ProcessService(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid service key %q: %w", key, err)
	}
	if deleted {
		h.correlator.ResolveByResource("service", namespace+"/"+name)
		return nil
	}
	svc, err := h.serviceLister.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("service", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get service %s/%s from cache: %w", namespace, name, err)
	}
	return h.ProcessServiceObject(svc, false)
}

func (h *handler) ProcessServiceObject(svc *corev1.Service, deleted bool) error {
	if svc == nil {
		return nil
	}
	if deleted {
		h.correlator.ResolveByResource("service", svc.Namespace+"/"+svc.Name)
		return nil
	}

	eps, err := h.endpointLister.Endpoints(svc.Namespace).Get(svc.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get endpoints %s/%s from cache: %w", svc.Namespace, svc.Name, err)
	}

	sig := DetectServiceEndpointIssue(svc, eps)
	if sig != nil {
		h.signalEvent(sig)
	} else {
		h.correlator.MarkResolved(
			correlation.BuildKey(svc.Namespace, svc.Namespace+"/"+svc.Name, "ServiceNoEndpoints", ""),
		)
	}
	return nil
}
