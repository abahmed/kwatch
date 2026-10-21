package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

func probeFinding(errorLine string) detection.Finding {
	ep := inventory.CoreID(explain.KindExternalEndpoint, "",
		"sandbox.partner-gateway.example:443")
	return detection.Finding{Entity: ep, Reason: reasons.ActiveProbeFailure,
		Severity: detection.Critical,
		Summary:  "Endpoint does not accept connections",
		Evidence: []detection.Evidence{{
			Label: detection.EvidenceError, Value: errorLine}}}
}

const nxdomainLine = "dial tcp: lookup sandbox.partner-gateway.example on " +
	"10.96.0.10:53: no such host"

// A name that does not exist was never connected to: the note must not say
// it refuses connections, whatever the probe recorded about how it failed.
func TestProbedEndpointWithNoSuchHostIsNotWordedAsRefusing(t *testing.T) {
	finding := probeFinding(nxdomainLine)
	p := incident.Incident{ID: "e", Root: finding.Entity,
		Tier: incident.Notify, State: incident.Open,
		Members: members(finding)}

	note := Writer{Cluster: "prod-eu-1"}.Write(announce(p), writerNow).Note

	if strings.Contains(note, "accept connections") ||
		!strings.Contains(note, "name that does not resolve") ||
		!strings.Contains(note, `"`+nxdomainLine+`"`) {
		t.Fatalf("note = %q", note)
	}
}

func TestProbedEndpointThatRefusesKeepsItsWords(t *testing.T) {
	finding := probeFinding("dial tcp 10.0.0.9:443: connection refused")
	if got := findingState(finding); got !=
		"does not accept connections" {
		t.Fatalf("state = %q", got)
	}
}
