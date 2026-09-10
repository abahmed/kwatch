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
	findings, err := DetectIngressIssueWithLookup(
		ing,
		func(ns, name string) (bool, error) {
			return hasService(ns, name), nil
		},
	)
	if err != nil {
		return nil
	}
	return findings
}

// DetectIngressIssueWithLookup is the error-aware detector used by live
// processing. NotFound is a missing backend; other errors are unknown.
func DetectIngressIssueWithLookup(
	ing *networkingv1.Ingress,
	lookup ServiceLookup,
) ([]*model.Observation, error) {
	if ing == nil || lookup == nil {
		return nil, nil
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
			exists, err := lookup(ns, svcName)
			if err != nil {
				return nil, fmt.Errorf(
					"lookup ingress backend Service %s/%s: %w",
					ns,
					svcName,
					err,
				)
			}
			if !exists {
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
		exists, err := lookup(ns, svcName)
		if err != nil {
			return nil, fmt.Errorf(
				"lookup ingress default backend Service %s/%s: %w",
				ns,
				svcName,
				err,
			)
		}
		if !exists {
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

	return sigs, nil
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

	findings, err := DetectIngressIssueWithLookup(
		ing,
		h.serviceLookup(),
	)
	if err != nil {
		return fmt.Errorf("evaluate ingress %s/%s: %w", ing.Namespace, ing.Name, err)
	}
	h.reconcile(subject, findings)
	return nil
}
