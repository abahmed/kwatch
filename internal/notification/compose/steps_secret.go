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
			// An app that reads it through the API names it nowhere
			// kwatch can see, so the step claims no more than that.
			Text: "Check that no pod or Ingress in its namespace " +
				"names the Secret (0 means none does; an app that " +
				"reads it through the API is not counted)",
			Command: "kubectl get ingress,pods" + ns +
				" -o yaml | grep -c -F " + name,
		}}
	}
	// A step never reads the Secret itself: its keys may be what the
	// reader must not see, and reading Secrets is a right many lack.
	// The pods' specs say which keys they expect from it.
	return []notification.Step{{
		Text: "See where the pods use the Secret and which keys " +
			"they expect from it",
		Command: "kubectl get pods -o yaml" + ns + " | grep -n -B2 -E " +
			secretUsePattern(root.Name),
	}}
}

// secretUsePattern is a grep -E pattern for the lines of a pod spec
// that name the Secret: a volume's secretName, or the name of an
// envFrom, secretKeyRef or image pull reference, whose key is printed
// just above it.
func secretUsePattern(secret string) string {
	return quote("(name|secretName): " +
		strings.ReplaceAll(secret, ".", `\.`) + "$")
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

// unusedLeftover reports the finding of an expired certificate that no
// pod or Ingress uses: a warning, where one in use is critical.
func unusedLeftover(members []detection.Finding) bool {
	for _, f := range members {
		if f.Reason == reasons.TLSCertExpired &&
			f.Severity < detection.Critical {
			return true
		}
	}
	return false
}
