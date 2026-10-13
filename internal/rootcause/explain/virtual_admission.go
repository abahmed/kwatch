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
	// ModeWebhookTLS is a webhook whose calls fail the TLS handshake:
	// the API server does not trust, or cannot verify, its certificate.
	ModeWebhookTLS detection.Mode = "Webhook.TLS"
	// ModeWebhookDenied is a webhook that answered and denied the
	// request: a policy decision about one object, not an outage.
	ModeWebhookDenied detection.Mode = "Webhook.Denied"
)

// failedCall finds the webhook the API server failed to call, in its
// own words: failed calling webhook "x.example.com": failed to call
// webhook: ... A webhook that answers and denies a request is worded
// "admission webhook ... denied the request" and is not a failed call.
var failedCall = regexp.MustCompile(`failed calling webhook "([^"]+)"`)

// callTimeout is a failed call that ran out of time.
var callTimeout = regexp.MustCompile(
	`deadline exceeded|timeout|timed out`)

// timeoutQuery is the "?timeout=10s" the API server adds to every
// webhook URL. It is the limit it set, not proof the call ran out of it.
var timeoutQuery = regexp.MustCompile(`\?timeout=[^\s"]*`)

// noEndpoints is a call to a webhook Service with nothing behind it.
const noEndpoints = "no endpoints available"

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
	names := attrList(e, kube.AttrWebhookNames)
	calls := failedCall.FindAllStringSubmatchIndex(text, -1)
	for i, m := range calls {
		if !containsFold(names, text[m[2]:m[3]]) {
			continue
		}
		end := callEnd(text, m[1], calls, i)
		if mode, ok := callMode(text[m[1]:end]); ok {
			return pseudoFailing(mode)
		}
	}
	if deniedBy(text, names) {
		return pseudoFailing(ModeWebhookDenied)
	}
	return nil
}

// callEnd is where the call at calls[i] stops: at the next failed call
// or the next webhook named in the error, so a later denial's wording
// (a reason that says "timeout") is never read as this call's failure.
func callEnd(text string, from int, calls [][]int, i int) int {
	end := len(text)
	if i+1 < len(calls) {
		end = calls[i+1][0]
	}
	if n := namedWebhook.FindStringIndex(text[from:]); n != nil &&
		from+n[0] < end {
		end = from + n[0]
	}
	return end
}

// deniedBy reports a denial from one of the configuration's webhooks:
// it answered, so it was called whatever its failure policy says.
func deniedBy(text string, names []string) bool {
	for _, m := range namedWebhook.FindAllStringSubmatch(text, -1) {
		if m[2] != "" && containsFold(names, m[1]) {
			return true
		}
	}
	return false
}

// callMode classifies the text of one failed webhook call.
func callMode(segment string) (detection.Mode, bool) {
	segment = timeoutQuery.ReplaceAllString(segment, "")
	if hasSignal(SignalTLS, segment) {
		return ModeWebhookTLS, true
	}
	if callTimeout.MatchString(segment) {
		return ModeWebhookTimeout, true
	}
	if hasSignal(SignalConnection, segment) ||
		strings.Contains(segment, noEndpoints) {
		return ModeWebhookCallFailed, true
	}
	return "", false
}

// otherQuota reports whether the error names a quota other than id.
func (v *view) otherQuota(id inventory.EntityID, text string) bool {
	return len(namesIn(text, namedQuota)) > 0 &&
		!namesObject(text, id.Name, namedQuota)
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
	names := attrList(e, kube.AttrWebhookNames)
	policies := attrList(e, kube.AttrFailurePolicy)
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

// Pseudo modes of the built-in admission checks besides webhooks and
// quotas, read from the failed creates that quote them.
const (
	// ModePodSecurityDenied is a namespace whose Pod Security
	// Admission enforce label refused a pod.
	ModePodSecurityDenied detection.Mode = "Admission.PodSecurity"
	// ModeLimitRangeDenied is a LimitRange whose bounds refused a pod.
	ModeLimitRangeDenied detection.Mode = "Admission.LimitRange"
	// ModePolicyDenied is a ValidatingAdmissionPolicy that denied a
	// request.
	ModePolicyDenied detection.Mode = "Admission.Policy"
)

// kindAdmissionPolicy is the entity kind of a ValidatingAdmissionPolicy.
var kindAdmissionPolicy = kube.KindFor("ValidatingAdmissionPolicy")

// podSecurityEnforce is the label that makes a namespace refuse pods.
const podSecurityEnforce = "pod-security.kubernetes.io/enforce="

// namedPolicy finds the policy that denied a request: "ValidatingAdmission
// Policy 'name' with binding 'binding' denied request: ...".
var namedPolicy = regexp.MustCompile(
	`admissionpolicy '([^']+)' with binding '[^']*' denied request`)

// A namespace is reached through containment, the rest by admission.
func admitsOrContains(link LinkType, id inventory.EntityID) bool {
	return link == LinkAdmits ||
		(link == LinkContains && id.Kind == kube.KindNamespace)
}

// podSecurityModes: a namespace whose enforce label refused a pod, seen
// from a controller whose creates it refuses: the effect's own error
// must quote the check.
func (v *view) podSecurityModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if admitsOrContains(link, id) &&
		hasSignal(SignalPodSecurity, v.text(effect)) &&
		v.enforcesPodSecurity(id) {
		return pseudoFailing(ModePodSecurityDenied)
	}
	return nil
}

// limitRangeModes: a LimitRange whose bounds refused a pod.
func (v *view) limitRangeModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if admitsOrContains(link, id) &&
		hasSignal(SignalLimitRange, v.text(effect)) {
		return pseudoFailing(ModeLimitRangeDenied)
	}
	return nil
}

// policyModes: a ValidatingAdmissionPolicy that denied a request.
func (v *view) policyModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if admitsOrContains(link, id) &&
		deniedByPolicy(v.text(effect), id.Name) {
		return pseudoFailing(ModePolicyDenied)
	}
	return nil
}

// enforcesPodSecurity reports a namespace labelled to refuse pods.
func (v *view) enforcesPodSecurity(ns inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(ns)
	return ok && strings.Contains(attrText(e, kube.AttrLabels),
		podSecurityEnforce)
}

// deniedByPolicy reports a denial quoted by the named policy.
func deniedByPolicy(text, policy string) bool {
	return namesObject(text, policy, namedPolicy)
}

// policyAdmitHops lead from a controller that cannot create objects to
// the admission policies. The namespace (Pod Security Admission) is
// reached by containment and a LimitRange as a constraint of it.
func (v *view) policyAdmitHops(_ inventory.EntityID) []hop {
	return hopsTo(LinkAdmits, v.s.Model.Entities(kindAdmissionPolicy)...)
}
