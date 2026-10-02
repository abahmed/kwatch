package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Pseudo modes of a webhook configuration that the API server could not
// call, read from the failed creates that name one of its webhooks.
const (
	// ModeWebhookTimeout is a webhook whose calls time out: its
	// endpoints may be ready while its backend cannot answer in time.
	ModeWebhookTimeout detection.Mode = "Webhook.Timeout"
	// ModeWebhookCallFailed is a webhook whose calls fail another way:
	// refused, reset or a failed handshake.
	ModeWebhookCallFailed detection.Mode = "Webhook.CallFailed"
)

// failedCall finds the webhook the API server failed to call, in its
// own words: failed calling webhook "x.example.com": failed to call
// webhook: ... A webhook that answers and denies a request is worded
// "admission webhook ... denied the request" and is not a failed call.
var failedCall = regexp.MustCompile(`failed calling webhook "([^"]+)"`)

// callTimeout is a failed call that ran out of time.
var callTimeout = regexp.MustCompile(`deadline exceeded|timeout|timed out`)

// namedWebhook finds a webhook the API server names, as one it failed
// to call or one that denied the request.
var namedWebhook = regexp.MustCompile(
	`(?:failed calling webhook|admission webhook) "([^"]+)"( denied)?`)

// namedQuota finds the quota that refused a create: "forbidden:
// exceeded quota: compute, requested: ..." or "failed quota: compute:
// must specify limits.cpu".
var namedQuota = regexp.MustCompile(
	`(?:exceeded|failed) quota: ([a-z0-9][a-z0-9.-]*[a-z0-9])`)

// failurePolicyIgnore is the policy of a webhook that fails open.
const failurePolicyIgnore = "Ignore"

// admissionModes are the pseudo modes of a webhook configuration seen
// from a controller whose creates it blocks: the configuration fails
// when the error names one of its webhooks and says the call failed.
// Each call is classified by its own part of the text, up to the next
// failed call.
func (v *view) admissionModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkAdmits {
		return nil
	}
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return nil
	}
	text := strings.ToLower(v.text(effect))
	names := strings.Split(attrText(e, kube.AttrWebhookNames), ",")
	calls := failedCall.FindAllStringSubmatchIndex(text, -1)
	for i, m := range calls {
		if !containsFold(names, text[m[2]:m[3]]) {
			continue
		}
		end := len(text)
		if i+1 < len(calls) {
			end = calls[i+1][0]
		}
		if mode, ok := callMode(text[m[1]:end]); ok {
			return []modeHealth{{mode: mode, health: detection.Failing,
				pseudo: true}}
		}
	}
	return nil
}

// callMode classifies the text of one failed webhook call.
func callMode(segment string) (detection.Mode, bool) {
	if callTimeout.MatchString(segment) {
		return ModeWebhookTimeout, true
	}
	if hasSignal(SignalConnection, segment) {
		return ModeWebhookCallFailed, true
	}
	return "", false
}

// admitsElsewhere reports whether an admission candidate is ruled out
// by the effect's own words: the error names another webhook or quota,
// or the webhook fails open, so its failed call rejected nothing.
func (v *view) admitsElsewhere(id, effect inventory.EntityID) bool {
	text := strings.ToLower(v.text(effect))
	switch id.Kind {
	case kube.KindValidatingHook, kube.KindMutatingWebhook:
		return v.otherWebhook(id, text)
	case kube.KindQuota:
		names := namedQuota.FindAllStringSubmatch(text, -1)
		for _, m := range names {
			if strings.EqualFold(m[1], id.Name) {
				return false
			}
		}
		return len(names) > 0
	}
	return false
}

// otherWebhook reports whether a webhook configuration cannot be the
// one that rejected a create failing with text. When the error names
// webhooks, only a configuration holding one of them can be. A webhook
// that denied the request was called whatever its policy, while a
// failed call rejects only when the webhook fails closed.
func (v *view) otherWebhook(id inventory.EntityID, text string) bool {
	e, ok := v.s.Model.Entity(id)
	if !ok {
		return false
	}
	names := strings.Split(attrText(e, kube.AttrWebhookNames), ",")
	policies := strings.Split(attrText(e, kube.AttrFailurePolicy), ",")
	named := namedWebhook.FindAllStringSubmatch(text, -1)
	if len(named) == 0 {
		return failsOpen(policies, -1)
	}
	for _, m := range named {
		at := indexFold(names, m[1])
		if at >= 0 && (m[2] != "" || !failsOpen(policies, at)) {
			return false
		}
	}
	return true
}

// failsOpen reports whether the webhook at a position fails open, or,
// for -1, whether every webhook of the configuration does. A missing
// policy reads as Fail, the API server's default.
func failsOpen(policies []string, at int) bool {
	if at >= 0 {
		return at < len(policies) &&
			strings.EqualFold(policies[at], failurePolicyIgnore)
	}
	for _, policy := range policies {
		if !strings.EqualFold(policy, failurePolicyIgnore) {
			return false
		}
	}
	return true
}

// indexFold is the position of name in names, ignoring case, or -1.
func indexFold(names []string, name string) int {
	for i, candidate := range names {
		if candidate != "" && strings.EqualFold(candidate, name) {
			return i
		}
	}
	return -1
}

// containsFold reports whether names holds name, ignoring case.
func containsFold(names []string, name string) bool {
	return indexFold(names, name) >= 0
}
