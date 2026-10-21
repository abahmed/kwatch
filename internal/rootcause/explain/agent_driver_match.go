package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// registersDriver reports whether the pod is given the driver's name:
// in its own driver names, or else in the arguments of the workload
// that owns it. A registrar carries the name inside a socket path, so a
// whole path segment counts.
func (v *view) registersDriver(
	pod inventory.EntityID, driver string,
) bool {
	if e, ok := v.s.Model.Entity(pod); ok &&
		namesDriverIn(attrText(e, kube.AttrDriverNames), driver) {
		return true
	}
	owner := rootcause.TopOwner(v.s.Model, pod)
	if owner == pod {
		return false
	}
	e, ok := v.s.Model.Entity(owner)
	return ok && namesDriverIn(attrText(e, kube.AttrTemplateQuoted), driver)
}

// namesDriverIn reports a space-separated list with an entry that is
// the driver's name or a path or a qualified name with it as a segment.
func namesDriverIn(list, driver string) bool {
	for _, entry := range strings.Fields(strings.ToLower(list)) {
		if entry == driver || segmentNames(entry, driver) {
			return true
		}
	}
	return false
}

// segmentNames reports a path whose segment is the driver's name, bare
// or as the registration socket "<driver>-reg.sock".
func segmentNames(entry, driver string) bool {
	for _, segment := range strings.Split(entry, "/") {
		if segment == driver ||
			segment == driver+kube.DriverSocketSuffix {
			return true
		}
	}
	return false
}
