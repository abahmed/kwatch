package detection

import "strings"

// admissionRejections are the status reasons of a pod the kubelet
// rejected at admission. The kubelet records each as a Warning event and
// as the status reason of the pod, which it fails, so the Failed pod
// already shows them (detection/upstream: PodReasons). The table below
// and AdmissionReason both read this one list, so a reason is added to
// both at once.
var admissionRejections = []string{
	"OutOfcpu", "OutOfmemory", "OutOfpods", "OutOfephemeral-storage",
	"NodeAffinity", "NodeName", "NodePorts", "TaintToleration",
	"PodOSNotSupported", "PodOSSelectorNodeLabelDoesNotMatch",
	"TopologyAffinityError", "SysctlForbidden", "InvalidNodeInfo",
	"UnknownReason", "UnexpectedAdmissionError",
	"UnexpectedPredicateFailureType",
	"InitContainerRestartPolicyForbidden",
	"SupplementalGroupsPolicyNotSupported", "AppArmor",
	"PodFeatureUnsupported", "PodLevelResourcesNotSupported",
	"VolumeAttachmentLimitExceeded",
}

// admissionLifecycleReason is the status reason the kubelet gives pods
// of a node that is shutting down. Its pods fail without being at fault,
// like a disrupted pod, so it is a known state reason but not a
// rejection.
const admissionLifecycleReason = "NodeShutdown"

// insufficientResourcePrefix starts the status reason of a pod the
// kubelet rejected for lack of a resource: OutOfcpu, OutOfmemory,
// OutOfpods, OutOfephemeral-storage or OutOf<extended resource>
// (pkg/kubelet/lifecycle/predicate.go).
const insufficientResourcePrefix = "OutOf"

// admissionReasons are the pod state reasons the registry knows: every
// rejection and the node shutdown.
var admissionReasons = admissionTable()

func admissionTable() []ReasonInfo {
	out := make([]ReasonInfo, 0, len(admissionRejections)+1)
	for _, name := range admissionRejections {
		out = append(out, stateReason(name))
	}
	return append(out, stateReason(admissionLifecycleReason))
}

// AdmissionReason reports a pod status reason that means the kubelet
// refused to run the pod: one of the known rejections, or OutOf and the
// name of an extended resource.
func AdmissionReason(reason string) bool {
	for _, name := range admissionRejections {
		if reason == name {
			return true
		}
	}
	return strings.HasPrefix(reason, insufficientResourcePrefix) &&
		len(reason) > len(insufficientResourcePrefix)
}
