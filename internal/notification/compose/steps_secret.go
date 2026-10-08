package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// secretSteps names the step after what uses the Secret: an Ingress,
// nothing at all (an expired leftover), or, by default, the pods.
func secretSteps(
	root inventory.EntityID, members []detection.Finding,
) []notification.Step {
	ns := namespaceFlag(root.Namespace)
	name := quote(root.Name)
	ingresses, others := secretUsers(members)
	switch {
	case ingresses == 1 && others == 0:
		return []notification.Step{{
			Text: "See which hosts use the Secret",
			Command: "kubectl get ingress " + ingressName(members) +
				ns,
		}}
	case ingresses > 0 && others == 0:
		return []notification.Step{{
			Text:    "See which hosts use the Secret",
			Command: "kubectl get ingress" + ns,
		}}
	case ingresses == 0 && others == 0 && unusedLeftover(members):
		return []notification.Step{{
			// 0 means no Ingress or pod of the namespace names it.
			Text: "Check that nothing in its namespace names the " +
				"Secret (0 means nothing does)",
			Command: "kubectl get ingress,pods" + ns +
				" -o yaml | grep -c -F " + name,
		}}
	}
	return []notification.Step{{
		// describe lists each key and its size, never a value.
		Text:    "List the keys of the Secret the pods depend on",
		Command: "kubectl describe secret " + name + ns,
	}}
}

// secretUsers counts the Ingresses and the other objects the findings
// name as using the Secret.
func secretUsers(members []detection.Finding) (ingresses, others int) {
	for _, f := range members {
		for _, e := range f.Evidence {
			if e.Label != detection.EvidenceUsedBy {
				continue
			}
			if strings.HasPrefix(e.Value, string(kube.KindIngress)+"/") {
				ingresses++
			} else {
				others++
			}
		}
	}
	return ingresses, others
}

// ingressName is the name of the first Ingress using the Secret.
func ingressName(members []detection.Finding) string {
	prefix := string(kube.KindIngress) + "/"
	for _, f := range members {
		for _, e := range f.Evidence {
			if e.Label == detection.EvidenceUsedBy &&
				strings.HasPrefix(e.Value, prefix) {
				return quote(strings.TrimPrefix(e.Value, prefix))
			}
		}
	}
	return ""
}

// unusedLeftover reports the finding of an expired certificate that
// nothing references: a warning, where one in use is critical.
func unusedLeftover(members []detection.Finding) bool {
	for _, f := range members {
		if f.Reason == reasons.TLSCertExpired &&
			f.Severity < detection.Critical {
			return true
		}
	}
	return false
}
