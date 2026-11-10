package compose

import (
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A Community message says what is failing and only the category of
// its cause: "Most likely cause: a config change." The category never names
// the object, the change, the version, the image or who made it; the
// Pro line says what Pro adds (which one, the proof, the fix).

// Cause categories, in plain words.
const (
	categoryConfigChange  = "a config change"
	categoryMissingConfig = "a missing Secret or ConfigMap"
	categoryNode          = "a node problem"
	categoryImagePull     = "an image pull problem"
	categoryImage         = "an image problem"
	categoryMemoryLimit   = "its memory limit"
	categoryResourceLimit = "a resource limit"
	categoryShortage      = "a resource shortage"
	categoryRollout       = "a recent rollout"
	categoryDependency    = "a dependency"
	categoryMissingDep    = "a missing dependency"
	categoryExternal      = "an external dependency"
	categoryScheduling    = "a scheduling problem"
	categoryStorage       = "a storage problem"
	categoryNetwork       = "a network problem"
	categoryDNS           = "a DNS problem"
	categoryPermissions   = "a permissions problem"
	categoryAccount       = "a missing ServiceAccount"
	categoryAdmission     = "an admission rule"
	categoryControlPlane  = "a control plane problem"
	categoryController    = "a controller problem"
	categoryFinalizer     = "a stuck finalizer"
	categoryProbe         = "a probe setting"
	categoryHelper        = "a helper container"
	categoryCertificate   = "an expired certificate"
	categoryScaling       = "an autoscaling limit"
	categoryBudget        = "a disruption budget"
	categoryNamespace     = "a namespace being deleted"
	categorySharedError   = "an error several workloads share"
	// categoryOther is the category of a row this table does not know.
	categoryOther = "a related object"
)

// causeCategories are keyed by the explain row that blamed the cause
// (rootcause.CauseRecord.Rule). "self" has none: a workload failing on
// its own has no cause to tell. The Secret and ConfigMap rows depend on
// the mode as well (see causeCategory).
var causeCategories = map[string]string{
	"account-has-no-grant":             categoryPermissions,
	"account-missing-at-create":        categoryAccount,
	"admission-policy-denies":          categoryAdmission,
	"apiserver-unavailable":            categoryControlPlane,
	"apiservice-backend-missing":       categoryControlPlane,
	"apiservice-unavailable":           categoryControlPlane,
	"autoscaling-limit":                categoryScaling,
	"autoscaling-limit-unavailable":    categoryScaling,
	"backends-failing":                 categoryDependency,
	"backends-not-ready":               categoryDependency,
	"budget-blocks-drain":              categoryBudget,
	"called-service-backend-changed":   categoryDependency,
	"called-service-backends-failing":  categoryDependency,
	"called-service-no-endpoints":      categoryDependency,
	"certificate-expired":              categoryCertificate,
	"claim-no-storage-class":           categoryStorage,
	"claim-not-usable":                 categoryStorage,
	"claim-pins-pod":                   categoryStorage,
	"cluster-dns-failing":              categoryDNS,
	"cluster-dns-server-workload":      categoryDNS,
	"cluster-dns-servers":              categoryDNS,
	"config-missing-or-changed":        categoryMissingConfig,
	"configmap-missing-or-changed":     categoryMissingConfig,
	"controller-failing":               categoryController,
	"controller-failing-condition":     categoryController,
	"custom-resource-failing":          categoryController,
	"ephemeral-limit-too-low":          categoryResourceLimit,
	"etcd-unavailable":                 categoryControlPlane,
	"external-endpoint-failing":        categoryExternal,
	"external-endpoint-unreachable":    categoryExternal,
	"finalizer-handler-stopped":        categoryFinalizer,
	"finalizer-unhandled":              categoryFinalizer,
	"helper-container-blocks-pod":      categoryHelper,
	"image-drift":                      categoryImage,
	"init-waits-on-service":            categoryDependency,
	"lease-holder-failing":             categoryController,
	"limit-range-rejects":              categoryAdmission,
	"liveness-kills-before-ready":      categoryProbe,
	"log-names-dependency":             categoryDependency,
	"liveness-shorter-than-start":      categoryProbe,
	"memory-limit-too-low":             categoryMemoryLimit,
	"metrics-api-down":                 categoryControlPlane,
	"missing-service-called":           categoryMissingDep,
	"missing-service-configured":       categoryMissingDep,
	"namespace-terminating":            categoryNamespace,
	"node-agent-absent":                categoryNode,
	"node-agent-failing":               categoryNode,
	"node-agent-named-by-error":        categoryNode,
	"node-degraded":                    categoryNode,
	"node-disk-pressure":               categoryNode,
	"node-memory-pressure":             categoryNode,
	"node-memory-pressure-unready":     categoryNode,
	"node-network":                     categoryNode,
	"node-not-ready":                   categoryNode,
	"node-overcommitted":               categoryNode,
	"node-pid-pressure":                categoryNode,
	"node-removed":                     categoryNode,
	"node-removed-capacity":            categoryNode,
	"nodepool-failing":                 categoryNode,
	"own-change":                       categoryConfigChange,
	"owner-failing":                    categoryDependency,
	"pod-security-rejects":             categoryAdmission,
	"policy-blocks-call":               categoryNetwork,
	"policy-restricts":                 categoryNetwork,
	"preemptor":                        categoryScheduling,
	"probe-port-mismatch":              categoryProbe,
	"provisioner-down":                 categoryStorage,
	"provisioner-down-pending":         categoryStorage,
	"pull-without-secret":              categoryImagePull,
	"quota-exhausted":                  categoryShortage,
	"rbac-change":                      categoryPermissions,
	"registry-refuses":                 categoryImagePull,
	"resource-claim-unallocated":       categoryShortage,
	"rollout":                          categoryRollout,
	"routed-missing":                   categoryMissingDep,
	"run-configmap-missing-or-changed": categoryMissingConfig,
	"run-secret-missing-or-changed":    categoryMissingConfig,
	"scheduler-capacity":               categoryScheduling,
	"scheduler-unavailable":            categoryControlPlane,
	"service-no-endpoints":             categoryDependency,
	"shared-failure-signature":         categorySharedError,
	"shared-node":                      categoryNode,
	"startup-budget-too-short":         categoryProbe,
	"storage-class-missing":            categoryStorage,
	"storage-class-missing-pending":    categoryStorage,
	"summary":                          categoryOther,
	"used-missing":                     categoryMissingDep,
	"volume-held-by-node":              categoryStorage,
	"volume-held-by-pod":               categoryStorage,
	"webhook-backend-missing":          categoryAdmission,
	"webhook-rejects":                  categoryAdmission,
	"webhook-slows-api":                categoryControlPlane,
	"workload-fills-disks":             categoryShortage,
	"zone-failing":                     categoryNode,

	// The longest rule name stands apart so the table keeps its layout.
	"called-service-backends-not-ready": categoryDependency,
}

// whichCategories are the categories that stand for one object among
// several, so the Pro line can say "Pro shows which one". For the rest
// it says "the exact cause".
var whichCategories = map[string]bool{
	categoryConfigChange: true, categoryMissingConfig: true,
	categoryNode: true, categoryRollout: true, categoryDependency: true,
	categoryMissingDep: true, categoryExternal: true,
	categoryAdmission: true, categoryController: true,
	categoryAccount: true,
}

// causeCategory is the category of the cause a Community message tells,
// or "" when there is nothing to tell: no cause, or a workload failing
// on its own.
func causeCategory(c *rootcause.CauseRecord) string {
	if c == nil || c.Rule == "self" {
		return ""
	}
	category, ok := causeCategories[c.Rule]
	if !ok {
		return categoryOther
	}
	if category == categoryMissingConfig && c.Mode != explain.ModeMissing {
		return categoryConfigChange
	}
	return category
}

// teaserLabel starts the sentence that tells the category, at the high
// and likely certainty levels: the Community message names no proof, so
// it never states the category as certain.
const teaserLabel = "Most likely cause: "

// teaserPossible starts the category of a cause below the likely level,
// the confidence wording of a diagnosis (see confidence.go).
const teaserPossible = "Possible cause: "

// causeTeaser is the sentence that tells the category of f's cause, at
// the cause's certainty: "Most likely cause: a config change." It is
// nil outside the Community message, without a category, and when the
// failing root is its own cause ("Node n1 is not ready"): the category
// would only repeat the lead.
func causeTeaser(f caseFacts) []sentence {
	if !f.community || causeIsRoot(f) {
		return nil
	}
	category := causeCategory(f.p.Cause)
	if category == "" {
		return nil
	}
	label := teaserLabel
	if certaintyOf(f.p.Cause) == certaintyPossible {
		label = teaserPossible
	}
	return []sentence{{part: partCause, fact: factCauseTeaser,
		text: label + category + "."}}
}

// causeIsRoot reports a cause that is the incident's root failing in its
// own right, which the lead already says.
func causeIsRoot(f caseFacts) bool {
	cause := f.p.Cause
	return cause != nil && cause.Root == f.p.Root &&
		rootFinding(f.p, f.members) != nil && !ownCause(cause) &&
		blamedChange(f) == nil
}

// What the Pro line of a message that tells its cause's category says
// Pro adds.
const (
	proWhich     = "which one"
	proExact     = "the exact cause"
	proProofTail = "the proof"
)

// causeNote is the one Pro line of a message that tells the category of
// cause: what Pro adds to it, and how to fix it or check it when Pro
// says so (help), else the proof.
func causeNote(cause *rootcause.CauseRecord, help proHelp) string {
	head, tail := proExact, proProofTail
	if whichCategories[causeCategory(cause)] {
		head = proWhich
	}
	switch help {
	case helpFix:
		tail = proFixWords
	case helpCheck:
		tail = proCheckWords
	}
	return proLineText([]string{head, tail})
}
