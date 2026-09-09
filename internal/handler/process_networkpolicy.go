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

// DetectNetworkPolicyIssue checks for potentially problematic NetworkPolicies.
// Currently detects deny-all egress policies that could silently break
// outbound connectivity.
func DetectNetworkPolicyIssue(
	policy *networkingv1.NetworkPolicy,
) *model.Observation {
	if policy == nil {
		return nil
	}
	// Check for overly restrictive default-deny policies
	isDenyAllEgress := len(policy.Spec.PolicyTypes) == 0 ||
		containsPolicyType(
			policy.Spec.PolicyTypes,
			networkingv1.PolicyTypeEgress,
		)
	if !isDenyAllEgress {
		return nil
	}

	// If no egress rules exist, it's a deny-all egress
	if len(policy.Spec.Egress) == 0 {
		return observe.Object(
			"networkpolicy", policy, constant.ReasonRestrictiveNetworkPolicy,
		).WithHint(fmt.Sprintf(
			"networkpolicy %s/%s has deny-all egress — may block outbound "+
				"connectivity",
			policy.Namespace,
			policy.Name,
		))
	}
	return nil
}

func containsPolicyType(
	types []networkingv1.PolicyType,
	t networkingv1.PolicyType,
) bool {
	for _, pt := range types {
		if pt == t {
			return true
		}
	}
	return false
}

func (h *handler) ProcessNetworkPolicy(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid networkpolicy key %q: %w", key, err)
	}
	if deleted {
		h.reconcileGone(model.NewObjectRef("networkpolicy", namespace, name))
		return nil
	}
	policy, err := h.listers.Netpol.NetworkPolicies(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(
				model.NewObjectRef("networkpolicy", namespace, name),
			)
			return nil
		}
		return fmt.Errorf(
			"failed to get networkpolicy %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}
	return h.ProcessNetworkPolicyObject(policy, false)
}

func (h *handler) ProcessNetworkPolicyObject(
	policy *networkingv1.NetworkPolicy,
	deleted bool,
) error {
	if policy == nil {
		return nil
	}
	subject := model.NewObjectRef(
		"networkpolicy", policy.Namespace, policy.Name,
	)
	if deleted {
		h.reconcileGone(subject)
		return nil
	}
	h.reconcile(subject, findings(DetectNetworkPolicyIssue(policy)))
	return nil
}
