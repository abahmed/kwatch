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
// that is a guess that the log then takes back.
func (v *view) configuredMayExplain(
	pod inventory.EntityID, quoted []string, service inventory.EntityID,
) bool {
	if len(quoted) == 0 {
		return !v.crashedWithError(pod)
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
