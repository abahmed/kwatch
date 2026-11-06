package explain

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A probe aimed at a port nothing listens on fails to connect. Ports
// in a container's spec are informational: an app may listen on one it
// does not declare (a health port given as a flag). So an undeclared
// probe port blames the probe only when the probe could not connect;
// a probe that got an HTTP status, or timed out after connecting,
// reached something that answers.

// probeNoListener is a probe failure that found nothing listening.
var probeNoListener = regexp.MustCompile(`(?i)connection refused|` +
	`no route to host|connect: (cannot assign|network is unreachable)|` +
	`dial tcp [^ ]+: connect:`)

// probeAnswered is a probe failure the target answered.
var probeAnswered = regexp.MustCompile(`(?i)statuscode|status code|` +
	`http probe failed with|returned [0-9]{3}`)

// probeRefused reports a container (or its pod, failing) whose probe
// failures say nothing listened, and none says the target answered.
func (v *view) probeRefused(ids ...inventory.EntityID) bool {
	refused := false
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			for _, e := range f.Evidence {
				if probeAnswered.MatchString(e.Value) {
					return false
				}
				refused = refused || probeNoListener.MatchString(e.Value)
			}
		}
	}
	return refused
}
