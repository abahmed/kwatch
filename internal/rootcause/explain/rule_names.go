package explain

// Names of the rows other packages branch on. A cause record carries
// its row's name (rootcause.CauseRecord.Rule), and messages, incidents
// and pages choose their words by it. The rows themselves use these
// constants, so renaming a row renames every branch on it, and a test
// keeps each constant the name of exactly one row. Code outside this
// package compares a rule with these constants, never with a string.
const (
	RuleSelf                         = "self"
	RuleRollout                      = "rollout"
	RuleOwnChange                    = "own-change"
	RuleConfigMissingOrChanged       = "config-missing-or-changed"
	RuleConfigMapMissingOrChanged    = "configmap-missing-or-changed"
	RuleRunConfigMapMissingOrChanged = "run-configmap-missing-or-changed"
	RuleRunSecretMissingOrChanged    = "run-secret-missing-or-changed"
	RuleFinalizerUnhandled           = "finalizer-unhandled"
	RuleFinalizerHandlerStopped      = "finalizer-handler-stopped"
	RuleExternalEndpointUnreachable  = "external-endpoint-unreachable"
	RuleExternalEndpointFailing      = "external-endpoint-failing"
	RulePolicyBlocksCall             = "policy-blocks-call"
	RuleMissingServiceCalled         = "missing-service-called"
	RuleMissingServiceConfigured     = "missing-service-configured"
	RuleAccountMissingAtCreate       = "account-missing-at-create"
	RuleCalledServiceBackendsFailing = "called-service-backends-failing"
	RuleClaimNoStorageClass          = "claim-no-storage-class"
	RuleStorageClassMissing          = "storage-class-missing"
	RuleStorageClassMissingPending   = "storage-class-missing-pending"
	RuleProvisionerDown              = "provisioner-down"
	RuleProvisionerDownPending       = "provisioner-down-pending"
	RuleNodeMemoryPressure           = "node-memory-pressure"
	RuleNodeOvercommitted            = "node-overcommitted"
	RuleVolumeHeldByPod              = "volume-held-by-pod"
	RuleVolumeHeldByNode             = "volume-held-by-node"
	RuleWebhookRejects               = "webhook-rejects"
	RuleLivenessShorterThanStart     = "liveness-shorter-than-start"
	RuleLivenessKillsBeforeReady     = "liveness-kills-before-ready"
	RuleHelmReleaseFailed            = "helm-release-failed"
)
