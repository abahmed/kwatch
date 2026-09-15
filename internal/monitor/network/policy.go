package network

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	networkingv1 "k8s.io/api/networking/v1"
)

// DetectNetworkPolicyIssue reports deny-all egress policies.
func DetectNetworkPolicyIssue(
	policy *networkingv1.NetworkPolicy,
) *model.Observation {
	if policy == nil {
		return nil
	}
	denyAllEgress := len(policy.Spec.PolicyTypes) == 0 ||
		containsPolicyType(
			policy.Spec.PolicyTypes,
			networkingv1.PolicyTypeEgress,
		)
	if !denyAllEgress || len(policy.Spec.Egress) != 0 {
		return nil
	}
	return observe.Object(
		"networkpolicy", policy, constant.ReasonRestrictiveNetworkPolicy,
	).WithHint(fmt.Sprintf(
		"networkpolicy %s/%s has deny-all egress — may block outbound "+
			"connectivity",
		policy.Namespace,
		policy.Name,
	))
}

func containsPolicyType(
	types []networkingv1.PolicyType,
	wanted networkingv1.PolicyType,
) bool {
	for _, policyType := range types {
		if policyType == wanted {
			return true
		}
	}
	return false
}
