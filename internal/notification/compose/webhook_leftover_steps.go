package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// matchesNothing is how the webhook detector says a configuration's
// rules match no object that exists now.
const matchesNothing = "rules match no object that exists now"

// webhookLeftoverSteps remove an admission webhook configuration left
// behind by a removed controller: nothing serves it (its Service does
// not exist or has no ready pods) and nothing needs it, which the
// detector proves when its rules match no object that exists now or
// marks the finding a leftover of the controller's absent backend. A
// missing Service alone is not enough: the controller may be on its way
// back, and its checks guard objects that still exist. The step first
// shows which Service each of its webhooks calls, then deletes the
// configuration, labelled as a change and saying what stops. It is
// offered only for a cause at least likely (undoSure); nil otherwise.
func webhookLeftoverSteps(
	p incident.Incident, members []detection.Finding,
) []notification.Step {
	root := p.Root
	if !webhookKind(root.Kind) || !undoSure(p.Cause) ||
		!leftoverWebhook(root.Name, members) {
		return nil
	}
	ref := string(root.Kind) + " " + quote(root.Name)
	return []notification.Step{{
		Text: "See the Service each of its webhooks calls",
		Command: "kubectl get " + ref +
			" -o jsonpath='{.webhooks[*].clientConfig.service}'",
	}, {
		Text: "Delete the leftover webhook configuration: nothing " +
			"serves it, and its checks or changes stop applying; " +
			"reinstall the controller instead if it is still wanted " +
			"(changes the cluster)",
		Command:  "kubectl delete " + ref,
		Mutating: true,
	}}
}

// leftoverWebhook reports a finding of the configuration name that
// says nothing serves it (its Service is gone or has no ready pods) and
// nothing needs it: its rules match nothing, or it is a leftover of
// its controller's absent backend.
func leftoverWebhook(name string, members []detection.Finding) bool {
	for _, m := range members {
		if m.Entity.Name != name || !webhookKind(m.Entity.Kind) {
			continue
		}
		dead := m.Reason == reasons.WebhookBackendNotFound ||
			m.Reason == reasons.WebhookNoEndpoints
		unneeded := strings.Contains(m.Summary, matchesNothing) ||
			!zeroID(m.Leftover)
		if dead && unneeded {
			return true
		}
	}
	return false
}
