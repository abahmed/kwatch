package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// EvidenceReadinessGate labels an unmet pod readiness gate, valued
// "type=status (reason)". A pod stays NotReady until every gate is True,
// even when all its containers are ready.
const EvidenceReadinessGate = "readiness gate"

// unmetReadinessGates names the gates in spec.readinessGates whose pod
// condition is not True. A gate nobody has set yet reads "Missing".
func unmetReadinessGates(e inventory.Entity) []detection.Evidence {
	declared := text(e, kube.AttrReadinessGates)
	if declared == "" {
		return nil
	}
	var out []detection.Evidence
	for _, gate := range strings.Split(declared, ",") {
		status, reason, _ := condition(e, gate)
		if status == "True" {
			continue
		}
		if status == "" {
			status = "Missing"
		}
		value := gate + "=" + status
		if reason != "" {
			value += " (" + reason + ")"
		}
		out = append(out, detection.Evidence{
			Label: EvidenceReadinessGate, Value: value})
	}
	return out
}

// withReadinessGates adds unmet gates to a NotReady finding.
func withReadinessGates(s *detection.Finding, e inventory.Entity) {
	gates := unmetReadinessGates(e)
	if len(gates) == 0 {
		return
	}
	s.Evidence = append(s.Evidence, gates...)
	s.Summary += "; a readiness gate is not met"
}
