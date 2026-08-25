package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
)

// DetectIngressIssue checks an Ingress for backends referencing non-existent services.
func DetectIngressIssue(ing *networkingv1.Ingress, hasService func(ns, name string) bool) []*event.Signal {
	var sigs []*event.Signal
	ns := ing.Namespace

	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			svcName := path.Backend.Service.Name
			if !hasService(ns, svcName) {
				sigs = append(sigs, &event.Signal{
					Resource:  "ingress",
					Namespace: ing.Namespace,
					Reason:    constant.ReasonIngressBackendNotFound,
					Owner:     ing.Namespace + "/" + ing.Name,
					PodName:   ing.Name,
					Labels:    ing.Labels,
					Hint:      fmt.Sprintf("ingress %s/%s: backend service %q not found", ing.Namespace, ing.Name, svcName),
				})
			}
		}
	}

	// Also check default backend
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		svcName := ing.Spec.DefaultBackend.Service.Name
		if !hasService(ns, svcName) {
			sigs = append(sigs, &event.Signal{
				Resource:  "ingress",
				Namespace: ing.Namespace,
				Reason:    constant.ReasonIngressBackendNotFound,
				Owner:     ing.Namespace + "/" + ing.Name,
				PodName:   ing.Name,
				Labels:    ing.Labels,
				Hint:      fmt.Sprintf("ingress %s/%s: default backend service %q not found", ing.Namespace, ing.Name, svcName),
			})
		}
	}

	return sigs
}

func (h *handler) ProcessIngress(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid ingress key %q: %w", key, err)
	}
	if deleted {
		h.correlator.ResolveByResource("ingress", namespace+"/"+name)
		return nil
	}
	ing, err := h.listers.ingress.Ingresses(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("ingress", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get ingress %s/%s from cache: %w", namespace, name, err)
	}
	return h.ProcessIngressObject(ing, false)
}

func (h *handler) ProcessIngressObject(ing *networkingv1.Ingress, deleted bool) error {
	if ing == nil {
		return nil
	}
	if deleted {
		h.correlator.ResolveByResource("ingress", ing.Namespace+"/"+ing.Name)
		return nil
	}

	hasService := func(ns, name string) bool {
		if h.listers.service == nil {
			return true
		}
		_, err := h.listers.service.Services(ns).Get(name)
		return err == nil
	}

	sigs := DetectIngressIssue(ing, hasService)
	for _, sig := range sigs {
		h.signalEvent(sig)
	}
	if len(sigs) == 0 {
		h.correlator.MarkResolved(correlation.BuildKey(ing.Namespace, ing.Namespace+"/"+ing.Name, constant.ReasonIngressBackendNotFound, ""))
	}
	return nil
}
