package explain

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// groupVersionPattern is a "group/version" the API server names when it
// cannot discover or clean up an API: group 1, version 2.
var groupVersionPattern = regexp.MustCompile(
	`([a-z0-9][a-z0-9.-]*\.[a-z0-9.-]*[a-z0-9])/(v[0-9]+[a-z0-9]*)`)

// namedAPIServiceHops lead from a namespace stuck deleting, whose
// condition names an API group version it cannot discover, to the
// APIService "<version>.<group>" that serves it, when that APIService
// is failing: the namespace waits on an API nothing answers.
func (v *view) namedAPIServiceHops(id inventory.EntityID) []hop {
	if id.Kind != kube.KindNamespace {
		return nil
	}
	var out []hop
	seen := map[inventory.EntityID]bool{}
	for _, m := range groupVersionPattern.FindAllStringSubmatch(
		v.text(id), -1) {
		v.gate(id, scanOf(kube.KindAPIService))
		for _, api := range v.s.Model.Entities(kube.KindAPIService) {
			if api.Name != m[2]+"."+m[1] || seen[api] {
				continue
			}
			v.gate(id, api)
			for _, f := range v.s.Findings[api] {
				if f.Mode.Within(detection.ModeAPIServiceUnavailable) {
					seen[api] = true
					out = append(out, hop{link: LinkServedBy, to: api})
					break
				}
			}
		}
	}
	return out
}
