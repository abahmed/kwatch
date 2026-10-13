package detection

import (
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withPlace adds the setting a container failure comes from, when the
// failure is the setting being reached: a memory limit that killed the
// container, or a CPU limit that throttles it while its probe fails.
// The object that holds the setting is left to the reader: it is the
// workload the incident is about.
func withPlace(e inventory.Entity, f Finding) Finding {
	if e.ID.Kind != kube.KindContainer || f.Advisory || f.Place != nil {
		return f
	}
	name := containerPart(e.ID.Name)
	switch {
	case f.Reason == reasons.OOMKilled &&
		!hasEvidence(f, EvidenceKilledByNode):
		if limit, ok := numberAttr(e, kube.AttrMemoryLimit); ok && limit > 0 {
			f.Place = &Place{Container: name,
				Path: "resources.limits.memory", Value: MemoryText(limit)}
		}
	case hasEvidence(f, EvidenceCPUThrottled):
		if limit, ok := numberAttr(e, kube.AttrCPULimit); ok && limit > 0 {
			f.Place = &Place{Container: name,
				Path: "resources.limits.cpu", Value: CPUText(limit)}
		}
	}
	return f
}

func hasEvidence(f Finding, label string) bool {
	for _, e := range f.Evidence {
		if e.Label == label {
			return true
		}
	}
	return false
}

func numberAttr(e inventory.Entity, name string) (float64, bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return 0, false
	}
	return attribute.Value.AsNumber()
}

// containerPart is the container's own name: what follows the slash in
// its "<pod>/<container>" entity name.
func containerPart(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[i+1:]
		}
	}
	return name
}
