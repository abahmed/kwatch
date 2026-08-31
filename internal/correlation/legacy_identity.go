package correlation

import (
	"strings"

	"github.com/abahmed/kwatch/internal/model"
)

// migrateLegacyPodKey bridges incidents written before ownerless Pods gained
// UID/lineage identities. Migration is deliberately accepted only when one
// current baseline entry matches the old incident's namespace, reason, and
// concrete Pod resource; ambiguity leaves the old incident untouched rather
// than risking a false merge.
func (e *Engine) migrateLegacyPodKey(key model.IncidentKey, inc *model.Incident) (model.IncidentKey, bool) {
	parts := ParseKey(key)
	if parts.IsGlobal || parts.IsGroup || parts.IsMassFailure || inc == nil || inc.Resource != "pod" {
		return "", false
	}
	var match model.IncidentKey
	for candidate, pods := range e.baseline {
		candidateParts := ParseKey(model.IncidentKey(candidate))
		if candidateParts.Namespace != parts.Namespace || candidateParts.Reason != parts.Reason {
			continue
		}
		if !strings.HasPrefix(candidateParts.Owner, "uid/") && !strings.HasPrefix(candidateParts.Owner, "lineage/") {
			continue
		}
		if _, found := pods[inc.Name]; !found && !containsIncidentResource(inc, pods) {
			continue
		}
		if match != "" {
			return "", false
		}
		match = model.IncidentKey(candidate)
	}
	return match, match != ""
}

func containsIncidentResource(inc *model.Incident, baselinePods map[string]int64) bool {
	for pod := range inc.Resources {
		if _, ok := baselinePods[pod]; ok {
			return true
		}
	}
	return false
}
