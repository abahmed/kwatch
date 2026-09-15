package network

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	networkingv1 "k8s.io/api/networking/v1"
)

// ServiceLookup reports whether a Service exists in a namespace.
type ServiceLookup func(namespace, name string) (bool, error)

// DetectIngressIssue adapts a boolean lookup for callers that cannot return
// cache errors.
func DetectIngressIssue(
	ing *networkingv1.Ingress,
	hasService func(namespace, name string) bool,
) []*model.Observation {
	if ing == nil || hasService == nil {
		return nil
	}
	findings, err := DetectIngressIssueWithLookup(
		ing,
		func(namespace, name string) (bool, error) {
			return hasService(namespace, name), nil
		},
	)
	if err != nil {
		return nil
	}
	return findings
}

// DetectIngressIssueWithLookup reports missing backend Services.
func DetectIngressIssueWithLookup(
	ing *networkingv1.Ingress,
	lookup ServiceLookup,
) ([]*model.Observation, error) {
	if ing == nil || lookup == nil {
		return nil, nil
	}
	var findings []*model.Observation
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			if err := appendMissingIngressService(
				&findings,
				ing,
				lookup,
				path.Backend.Service.Name,
				"backend",
			); err != nil {
				return nil, err
			}
		}
	}
	if ing.Spec.DefaultBackend != nil &&
		ing.Spec.DefaultBackend.Service != nil {
		if err := appendMissingIngressService(
			&findings,
			ing,
			lookup,
			ing.Spec.DefaultBackend.Service.Name,
			"default backend",
		); err != nil {
			return nil, err
		}
	}
	return findings, nil
}

func appendMissingIngressService(
	findings *[]*model.Observation,
	ing *networkingv1.Ingress,
	lookup ServiceLookup,
	serviceName, role string,
) error {
	exists, err := lookup(ing.Namespace, serviceName)
	if err != nil {
		return fmt.Errorf(
			"lookup ingress %s Service %s/%s: %w",
			role,
			ing.Namespace,
			serviceName,
			err,
		)
	}
	if exists {
		return nil
	}
	*findings = append(*findings, observe.Object(
		"ingress", ing, constant.ReasonIngressBackendNotFound,
	).WithHint(fmt.Sprintf(
		"ingress %s/%s: %s service %q not found",
		ing.Namespace,
		ing.Name,
		role,
		serviceName,
	)))
	return nil
}
