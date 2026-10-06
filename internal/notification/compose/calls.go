package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// endpointClassWords say how calls to an external endpoint fail, by
// the class explain put after "EndpointFailing.".
var endpointClassWords = map[string]string{
	"refused":     "refuses connections",
	"unresolved":  "does not resolve",
	"tls":         "fails the TLS handshake",
	"unreachable": "cannot be reached",
}

// endpointWords follow an external endpoint's name: "external endpoint
// db.example.com:5432 refuses connections".
func endpointWords(mode detection.Mode) string {
	_, class, _ := strings.Cut(string(mode), ".")
	if words, ok := endpointClassWords[strings.ToLower(class)]; ok {
		return words
	}
	return causeWords["external-endpoint-failing"].words
}

// signatureLead names a storm with nothing to blame but its words:
// "api in shop and four other workloads keep crashing with the same
// error." The error itself is quoted by the next sentence.
func signatureLead(f caseFacts) string {
	subject := symptomSubject(f)
	others := otherWorkloads(f, subject)
	state := stateWords(f.p)
	if f.ok {
		state = beforeColon(predicate(f.lead.Entity, f.lead.Summary))
	}
	if others == 0 {
		return f.leadName(subject) + " " + state +
			" with an error other workloads share"
	}
	return f.leadName(subject) + " and " + plural(others, "other workload") +
		" " + pluralPredicate(state) + " with the same error"
}

// otherWorkloads counts the workloads in the impact besides subject.
func otherWorkloads(f caseFacts, subject inventory.EntityID) int {
	n := 0
	for _, id := range f.p.Impact {
		if incident.IsWorkload(id.Kind) && id != subject {
			n++
		}
	}
	return n
}
