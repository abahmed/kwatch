package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// policyAdmitRows are the built-in admission checks that refuse the
// creates of a controller: Pod Security Admission, LimitRange bounds and
// ValidatingAdmissionPolicy denials. Each row needs the effect's own
// error to quote the check, so a healthy rule is never blamed.
var policyAdmitRows = []Row{
	{
		Name: "pod-security-rejects",
		Cause: Side{Kind: kube.KindNamespace,
			Modes: []detection.Mode{ModePodSecurityDenied}},
		Link: LinkContains,
		Effect: Side{Group: AnyGroup, Kind: AnyKind, Modes: createModes,
			Signal: SignalPodSecurity},
		Prior: 0.8,
	},
	{
		Name: "limit-range-rejects",
		Cause: Side{Kind: kube.KindLimitRange,
			Modes: []detection.Mode{ModeLimitRangeDenied}},
		Link: LinkAdmits,
		Effect: Side{Group: AnyGroup, Kind: AnyKind, Modes: createModes,
			Signal: SignalLimitRange},
		Prior: 0.8,
	},
	{
		Name: "admission-policy-denies",
		Cause: Side{Group: AnyGroup, Kind: kindAdmissionPolicy,
			Modes: []detection.Mode{ModePolicyDenied}},
		Link: LinkAdmits,
		Effect: Side{Group: AnyGroup, Kind: AnyKind, Modes: createModes,
			Signal: SignalAdmissionPolicy},
		Prior: 0.8,
	},
	{
		// The ServiceAccount admission controller refused the pods
		// because the account their template names is gone.
		Name: "account-missing-at-create",
		Cause: Side{Kind: kube.KindAccount,
			Modes: []detection.Mode{ModeMissing}},
		Link: LinkUses,
		Effect: Side{Group: AnyGroup, Kind: AnyKind, Modes: createModes,
			Signal: SignalNoAccount},
		Prior: 0.85,
	},
}
