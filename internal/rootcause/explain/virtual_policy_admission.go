package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

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

// policyAdmissionModes are the pseudo modes of a namespace, LimitRange
// or admission policy seen from a controller whose creates they refuse:
// the effect's own error must quote the check.
func (v *view) policyAdmissionModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	// The namespace is reached through containment, the rest by admission.
	if link != LinkAdmits && !(link == LinkContains &&
		id.Kind == kube.KindNamespace) {
		return nil
	}
	text := v.text(effect)
	mode := detection.Mode("")
	switch id.Kind {
	case kube.KindNamespace:
		if hasSignal(SignalPodSecurity, text) && v.enforcesPodSecurity(id) {
			mode = ModePodSecurityDenied
		}
	case kube.KindLimitRange:
		if hasSignal(SignalLimitRange, text) {
			mode = ModeLimitRangeDenied
		}
	case kindAdmissionPolicy:
		if deniedByPolicy(text, id.Name) {
			mode = ModePolicyDenied
		}
	}
	if mode == "" {
		return nil
	}
	return []modeHealth{{mode: mode, health: detection.Failing,
		pseudo: true}}
}

// enforcesPodSecurity reports a namespace labelled to refuse pods.
func (v *view) enforcesPodSecurity(ns inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(ns)
	return ok && strings.Contains(attrText(e, kube.AttrLabels),
		podSecurityEnforce)
}

// deniedByPolicy reports a denial quoted by the named policy.
func deniedByPolicy(text, policy string) bool {
	for _, m := range namedPolicy.FindAllStringSubmatch(
		strings.ToLower(text), -1) {
		if strings.EqualFold(m[1], policy) {
			return true
		}
	}
	return false
}

// policyAdmitHops lead from a controller that cannot create objects to
// the admission policies. The namespace (Pod Security Admission) is
// reached by containment and a LimitRange as a constraint of it.
func (v *view) policyAdmitHops() []hop {
	var out []hop
	for _, policy := range v.s.Model.Entities(kindAdmissionPolicy) {
		out = append(out, hop{link: LinkAdmits, to: policy})
	}
	return out
}
