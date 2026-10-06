package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// deniedCause reports a cause that is a healthy admission webhook
// denying creates: a policy decision, not an outage of the webhook.
func deniedCause(cause *rootcause.CauseRecord) bool {
	return cause != nil && cause.Mode == explain.ModeWebhookDenied &&
		webhookKind(cause.Root.Kind)
}

// deniedLead says what the policy blocks and why: "checkout in payments
// is blocked by admission policy image-policy: image r/checkout:unsigned
// has no signature".
func deniedLead(f caseFacts) string {
	text := f.leadName(symptomSubject(f)) +
		" is blocked by admission policy " + f.p.Cause.Root.Name
	if reason := denialReason(f); reason != "" {
		text += ": " + reason
	}
	return text
}

// denial matches the part of a denial's message that is the policy's
// own reason: `admission webhook "x" denied the request: <reason>`.
var denial = regexp.MustCompile(`denied the request:\s*(.+)`)

// denialReason is the reason in the first member message that carries
// one, without a closing period or quote. Empty when none does.
func denialReason(f caseFacts) string {
	for _, m := range f.members {
		texts := []string{m.Summary}
		for _, e := range m.Evidence {
			texts = append(texts, e.Value)
		}
		for _, text := range texts {
			if found := denial.FindStringSubmatch(text); found != nil {
				return clip(strings.Trim(strings.TrimSpace(found[1]), `."'`))
			}
		}
	}
	return ""
}
