package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// insufficientResourcePrefix starts the status reason of a pod the
// kubelet rejected for lack of a resource: OutOfcpu, OutOfmemory,
// OutOfpods, OutOfephemeral-storage or OutOf<extended resource>
// (pkg/kubelet/lifecycle/predicate.go).
const insufficientResourcePrefix = "OutOf"

// admissionReasons are the other status reasons of a pod the kubelet
// rejected at admission: its own predicate reasons, the scheduler plugin
// names it reports for a failed predicate, and the sysctl and topology
// manager admit handlers (pkg/kubelet/lifecycle/predicate.go,
// pkg/kubelet/sysctl, pkg/kubelet/cm/topologymanager).
var admissionReasons = map[string]bool{
	"InvalidNodeInfo":                      true,
	"PodOSSelectorNodeLabelDoesNotMatch":   true,
	"PodOSNotSupported":                    true,
	"InitContainerRestartPolicyForbidden":  true,
	"SupplementalGroupsPolicyNotSupported": true,
	"UnexpectedAdmissionError":             true,
	"UnexpectedPredicateFailureType":       true,
	"UnknownReason":                        true,
	"NodeAffinity":                         true,
	"NodeName":                             true,
	"NodePorts":                            true,
	"TaintToleration":                      true,
	"SysctlForbidden":                      true,
	"TopologyAffinityError":                true,
}

// admissionFinding reports a pod the kubelet refused to run. Its Mode
// names the rejection, such as "Admission.Rejected.OutOfcpu".
func admissionFinding(e inventory.Entity) (detection.Finding, bool) {
	reason := text(e, kube.AttrReason)
	if !admissionReasons[reason] &&
		!(strings.HasPrefix(reason, insufficientResourcePrefix) &&
			len(reason) > len(insufficientResourcePrefix)) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.PodAdmissionRejected, Severity: detection.Critical,
		Mode: detection.ModeFor(reasons.PodAdmissionRejected) +
			detection.Mode("."+reason),
		Since:   valueSince(e, kube.AttrPhase),
		Summary: "Kubelet rejected the pod at admission (" + reason + ")",
		Evidence: []detection.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}, true
}
