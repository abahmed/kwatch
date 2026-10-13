package detection

import "github.com/abahmed/kwatch/internal/detection/reasons"

// serviceReasons is the service, admission, storage and generic reasons.
var serviceReasons = []ReasonInfo{
	reason(reasons.ServiceNoEndpoints, ModeNoEndpoints),
	reason(reasons.ServiceBackendsDegraded, ModeBackendsDegraded),
	reason(reasons.LoadBalancerPending, ModeLoadBalancerPending),
	reason(reasons.IngressBackendNotFound, ModeBackendMissing),
	reason(reasons.WebhookBackendNotFound, ModeWebhookBackendMissing),
	reason(reasons.ServicePortMismatch, ModePortMismatch),
	reason(reasons.IngressBackendPortMissing, ModePortMismatch),
	reason(reasons.WebhookNoEndpoints, ModeWebhookNoEndpoints),
	reason(reasons.WebhookSlow, ModeWebhookSlow),
	reason(reasons.WebhookRejecting, ModeWebhookRejecting),
	reason(reasons.WebhookFailingOpen, ModeWebhookFailingOpen),
	reason(reasons.AdmissionPolicyInvalid, ModeInvalidPolicy),
	reason(reasons.AdmissionBindingInvalid, ModeInvalidPolicyBinding),
	reason(reasons.MutatingAdmissionPolicyInvalid, ModeInvalidPolicyMutating),
	reason(reasons.CertificateSigningRequestFailure, ModeCSRFailed),
	reason(reasons.APIPriorityAndFairnessFailure, ModeFlowControl),
	reason(reasons.ResourceClaimFailure, ModeClaimFailed),
	reason(reasons.PodSecurityPolicyInvalid, ModeInvalidPolicySecurity),
	reason(reasons.ServiceAccountMissing, ModeMissingServiceAccount),
	reason(reasons.ProjectedSecretMissing, ModeMissingSecret),
	reason(reasons.ProjectedConfigMapMissing, ModeMissingConfigMap),
	reason(reasons.APIServiceFailure, ModeAPIServiceUnavailable),
	reason(reasons.CustomResourceFailure, ModeNotReconciling),
	reason(reasons.RestrictiveNetworkPolicy, ModeNetworkPolicy),
	// Cause-only: these fire before anything failed (a volume nearly
	// full, an autoscaler at its maximum) yet are real causes of the
	// failures that follow. An incident made only of them is silent.
	reason(reasons.VolumeUsageHigh, ModeVolumeFull).causeOnly(),
	reason(reasons.VolumeInodesHigh, ModeVolumeInodes).causeOnly(),
	reason(reasons.PersistentVolumeClaim, ModeClaimFailed),
	reason(reasons.PersistentVolume, ModeVolumeFailed),
	reason(reasons.ResourceQuotaExhausted, ModeQuotaExhausted),
	reason(reasons.LimitRangeInvalid, ModeInvalidLimitRange),
	reason(reasons.NamespaceStuck, ModeStuckDeleting),
	reason(reasons.NodeStuckTerminating, ModeStuckDeleting),
	// UnusualEventPrefix is a prefix: the finding sets the mode itself.
	reason(reasons.UnusualEventPrefix, ModeUnusualEvent),
	// UnknownWaitingPrefix is a prefix too: the finding sets its mode.
	reason(reasons.UnknownWaitingPrefix, ModeUnknownWaiting),
	reason(reasons.ConditionFailure, ModeCondition),
	reason(reasons.GenerationLagging, ModeNotReconciling),
	reason(reasons.StuckDeleting, ModeStuckDeleting),
	reason(reasons.PhaseFailed, ModeFailed),
	reason(reasons.PhasePending, ModePending),
}
