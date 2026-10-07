package detectors

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// namespaceDeletionConditions are the conditions the namespace
// controller sets while it cannot finish a deletion, in the order that
// reads best: what is left, who holds it, and why it could not be read
// or deleted. Each is True while the problem lasts.
var namespaceDeletionConditions = []string{
	"NamespaceContentRemaining", "NamespaceFinalizersRemaining",
	"NamespaceDeletionContentFailure",
	"NamespaceDeletionDiscoveryFailure",
}

// stuckNamespace reports a namespace that has been terminating for too
// long. The messages of the controller's own conditions are quoted, not
// interpreted: they name what is left and which finalizers hold it.
func stuckNamespace(
	ctx detection.Context, e inventory.Entity, since time.Time,
) detection.Finding {
	var quoted []string
	var evidence []detection.Evidence
	if finalizers := text(e, kube.AttrFinalizers); finalizers != "" {
		evidence = append(evidence, detection.Evidence{
			Label: "finalizers", Value: finalizers})
	}
	for _, name := range namespaceDeletionConditions {
		status, _, _ := condition(e, name)
		message := text(e, kube.ConditionKey(name)+kube.AttrConditionMessage)
		if status != "True" || message == "" {
			continue
		}
		quoted = append(quoted, message)
		evidence = append(evidence,
			detection.Evidence{Label: name, Value: message})
	}
	summary := "Namespace has been terminating for " +
		format.Duration(ctx.Now.Sub(since))
	if len(quoted) == 0 {
		summary += "; a finalizer is likely blocking it"
	} else {
		summary += ": " + strings.Join(quoted, "; ")
	}
	return detection.Finding{
		Reason: reasons.NamespaceStuck, Severity: detection.Warning,
		Since: since, Summary: summary, Evidence: evidence,
	}
}
