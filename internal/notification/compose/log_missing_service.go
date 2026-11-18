package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// weightLogMissing ranks what kwatch checked above the generic proof:
// it is all there is to know about a workload Kubernetes sees healthy.
const weightLogMissing = 0.9

// logMissing returns the root's own finding when its log keeps failing
// to resolve a Service that does not exist.
func logMissing(
	p incident.Incident, members []detection.Finding,
) *detection.Finding {
	own := rootFinding(p, members)
	if own == nil || own.Mode != detection.ModeLogServiceMissing {
		return nil
	}
	return own
}

// missingNameOf splits the evidence "name.namespace".
func missingNameOf(own detection.Finding) (name, namespace string) {
	name, namespace, _ = strings.Cut(
		evidence(own, detection.EvidenceMissingService), ".")
	return name, namespace
}

// logMissingLead: "reports in shop keeps trying to reach Service
// geo, which does not exist".
func logMissingLead(f caseFacts) (string, bool) {
	own := logMissing(f.p, f.members)
	if own == nil {
		return "", false
	}
	return f.leadName(own.Entity) + " " + own.Summary, true
}

// logMissingSentences say what the log shows and what kwatch checked:
// "Its pods are Running and Ready; the log shows "…" in 5 samples.
// kwatch found no Service named geo in namespace shop; Service geo-api
// there has a similar name."
func logMissingSentences(f caseFacts) []sentence {
	own := logMissing(f.p, f.members)
	if own == nil {
		return nil
	}
	name, namespace := missingNameOf(*own)
	text := "Its pods are Running and Ready; the log shows " +
		quoted(evidence(*own, detection.EvidenceLogLine)) + " in " +
		evidence(*own, detection.EvidenceLogSamples) + " samples."
	checked := "kwatch found no Service named " + name +
		" in namespace " + namespace
	if near := evidence(*own, detection.EvidenceSimilarService); near != "" {
		checked += "; Service " + near + " there has a similar name"
	}
	s := proof(weightLogMissing, checked+".")
	s.keepCase = true
	return []sentence{{part: partProof, weight: weightLogMissing,
		text: text}, s}
}

// logMissingSteps list the Services of the namespace.
func logMissingSteps(own detection.Finding) []notification.Step {
	_, namespace := missingNameOf(own)
	return []notification.Step{{
		Text: "Compare the name with the Services of " + namespace +
			" and with " + own.Entity.Name + "'s configuration",
		Command: "kubectl get services" + namespaceFlag(namespace),
	}}
}
