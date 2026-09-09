package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectIngressIssue checks an Ingress for backends referencing non-existent
// services.
func DetectIngressIssue(
	ing *networkingv1.Ingress,
	hasService func(ns, name string) bool,
) []*model.Observation {
	if ing == nil || hasService == nil {
		return nil
	}
	var sigs []*model.Observation
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
				sigs = append(sigs, observe.Object(
					"ingress", ing, constant.ReasonIngressBackendNotFound,
				).WithHint(fmt.Sprintf(
					"ingress %s/%s: backend service %q not found",
					ing.Namespace,
					ing.Name,
					svcName,
				)))
			}
		}
	}

	// Also check default backend
	if ing.Spec.DefaultBackend != nil &&
		ing.Spec.DefaultBackend.Service != nil {
		svcName := ing.Spec.DefaultBackend.Service.Name
		if !hasService(ns, svcName) {
			sigs = append(sigs, observe.Object(
				"ingress", ing, constant.ReasonIngressBackendNotFound,
			).WithHint(fmt.Sprintf(
				"ingress %s/%s: default backend service %q not found",
				ing.Namespace,
				ing.Name,
				svcName,
			)))
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
		h.reconcileGone(model.NewObjectRef("ingress", namespace, name))
		return nil
	}
	ing, err := h.listers.Ingress.Ingresses(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(model.NewObjectRef("ingress", namespace, name))
			return nil
		}
		return fmt.Errorf(
			"failed to get ingress %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}
	return h.ProcessIngressObject(ing, false)
}

func (h *handler) ProcessIngressObject(
	ing *networkingv1.Ingress,
	deleted bool,
) error {
	if ing == nil {
		return nil
	}
	subject := model.NewObjectRef("ingress", ing.Namespace, ing.Name)
	if deleted {
		h.reconcileGone(subject)
		return nil
	}

	hasService := func(ns, name string) bool {
		if h.listers.Service == nil {
			return true
		}
		_, err := h.listers.Service.Services(ns).Get(name)
		return err == nil
	}

	h.reconcile(subject, DetectIngressIssue(ing, hasService))
	return nil
}
