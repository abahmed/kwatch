package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// unresolvedSummary is the detector's own words for a probed endpoint
// whose name does not resolve.
const unresolvedSummary = "Endpoint name does not resolve"

// leadState is what follows the lead finding's subject.
func leadState(f caseFacts) string { return findingState(f.lead) }

// findingState is what follows a finding's subject. An external endpoint
// whose probe error says "no such host" is worded as a name that does not
// resolve: the probe may not have recorded how it failed, and the generic
// "does not accept connections" would then say a connection was refused
// when none was made.
func findingState(f detection.Finding) string {
	if unresolvedProbe(f) {
		return predicate(f.Entity, unresolvedSummary)
	}
	return predicate(f.Entity, f.Summary)
}

// unresolvedProbe reports an external endpoint finding whose own error
// line says the host name does not exist.
func unresolvedProbe(f detection.Finding) bool {
	if f.Entity.Kind != explain.KindExternalEndpoint {
		return false
	}
	return strings.Contains(strings.ToLower(
		evidence(f, detection.EvidenceError)), "no such host")
}
