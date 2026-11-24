package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// configuredMayExplain reports whether a Service that only the pod's
// configuration names, and that does not exist, may explain its
// failure. The quoted errors must name it: a crash that quotes another
// error is its own cause, and a name in the configuration never
// outranks what the log says. A pod that quotes nothing may be
// explained by it only when it did not crash with an error: a crash
// whose log is not read yet will say why, and blaming the name before
// that is a guess that the log then takes back, and a pod that ran
// ready with the name already missing was not stopped by it
// (ranWithoutIt). A pod whose finding
// proves its own fault (a binary built for another CPU or not
// executable, a configuration it rejects or lacks; ownFaultProven) is
// never explained by a name its configuration holds.
func (v *view) configuredMayExplain(
	pod inventory.EntityID, quoted []string, service inventory.EntityID,
) bool {
	if v.ownFaultProven(pod) || v.errorsNameAnotherCall(pod, service) {
		return false
	}
	if len(quoted) == 0 {
		return !v.crashedWithError(pod) && !v.ranWithoutIt(pod, service)
	}
	for _, line := range quoted {
		if strings.Contains(strings.ToLower(line), service.Name) {
			return true
		}
	}
	return false
}

// crashedWithError reports a pod a container of which last ended with a
// non-zero exit code.
func (v *view) crashedWithError(pod inventory.EntityID) bool {
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		e, ok := v.s.Model.Entity(id)
		if !ok {
			continue
		}
		if _, known := e.Attribute(kube.AttrLastExitCode); !known {
			continue
		}
		if code, _ := attrNumber(e, kube.AttrLastExitCode); code != 0 {
			return true
		}
	}
	return false
}

// ownFaultProven reports a pod one of whose findings, or its
// containers', names its own fault: selfEvidentModes, or a binary built
// for another CPU than its node's. A fatal line alone is not one here:
// it may be the very line that names the missing Service.
func (v *view) ownFaultProven(pod inventory.EntityID) bool {
	ids := append([]inventory.EntityID{pod}, v.s.Model.Related(pod,
		inventory.PartOf, inventory.Incoming)...)
	for _, id := range ids {
		if v.hasMode(id, selfEvidentModes) ||
			archEvidence(v.s.Findings[id]) {
			return true
		}
	}
	return false
}

// errorsNameAnotherCall reports a pod whose own errors name another
// Service its configuration calls, and never service: the failure
// points at that dependency (a cache that went away), and a name only
// the configuration holds, which the pod ran with before, does not
// explain it. A Service is named by its name or, for a name like
// "redis-master", by its first word when that is long enough to mean
// something ("RedisConnectionException").
func (v *view) errorsNameAnotherCall(
	pod, service inventory.EntityID,
) bool {
	entity, ok := v.s.Model.Entity(pod)
	if !ok {
		return false
	}
	var texts []string
	for _, f := range v.unitFindings(pod) {
		for _, text := range v.evidenceTexts(f) {
			texts = append(texts, strings.ToLower(text))
		}
	}
	if namedIn(texts, service.Name) {
		return false
	}
	for _, call := range kube.ServiceCalls(entity) {
		if call.Service != service && namedIn(texts, call.Service.Name) {
			return true
		}
	}
	return false
}

// minNameWord is the shortest first word of a Service name that counts
// as naming it.
const minNameWord = 4

// namedIn reports a text that names the Service name.
func namedIn(texts []string, name string) bool {
	word, _, _ := strings.Cut(name, "-")
	for _, text := range texts {
		if strings.Contains(text, name) ||
			len(word) >= minNameWord && strings.Contains(text, word) {
			return true
		}
	}
	return false
}
