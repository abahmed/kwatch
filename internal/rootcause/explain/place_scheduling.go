package explain

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The scheduler's own names for what rejects a node. The root of a
// scheduling cause is named after the one that rejects most nodes.
const (
	schedTaint    = "untolerated taint"
	schedSelector = "node affinity/selector"
)

// schedulingPlace is the pod setting the scheduler rejects nodes by:
// the tolerations it lacks or the node selector it sets. Another
// reason (too little CPU, a volume, a spread rule) is capacity or
// topology, not a setting this names.
func (s Snapshot) schedulingPlace(c Cause) *detection.Place {
	reason := c.Root.Name
	for _, id := range c.Covers {
		if id.Kind != kube.KindPod {
			continue
		}
		pod, ok := s.Model.Entity(id)
		if !ok {
			continue
		}
		spec := kube.ParseSchedulingSpec(pod)
		switch {
		case strings.Contains(reason, schedTaint):
			return podSpecPlace(s.topOwner(id), "tolerations",
				tolerationText(spec.Tolerations))
		case strings.Contains(reason, schedSelector) &&
			len(spec.Selector) > 0:
			return podSpecPlace(s.topOwner(id), "nodeSelector",
				selectorText(spec.Selector))
		}
	}
	return nil
}

// podSpecPlace builds a place in the pod spec of the object that owns
// the pods: spec.template.spec.<field> for a workload.
func podSpecPlace(
	object inventory.EntityID, field, value string,
) *detection.Place {
	path := "spec.template.spec." + field
	switch object.Kind {
	case kube.KindPod:
		path = "spec." + field
	case kube.KindCronJob:
		path = "spec.jobTemplate.spec.template.spec." + field
	}
	return objectPlace(object, path, value)
}

// tolerationText lists tolerations as "key=value:Effect"; "none" when
// the pod has none.
func tolerationText(list []kube.Toleration) string {
	if len(list) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(list))
	for _, t := range list {
		part := t.Key
		if t.Operator != "Exists" && t.Value != "" {
			part += "=" + t.Value
		}
		if t.Effect != "" {
			part += ":" + t.Effect
		}
		parts = append(parts, part)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// selectorText lists a node selector as "key=value", sorted.
func selectorText(selector map[string]string) string {
	parts := make([]string, 0, len(selector))
	for key, value := range selector {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
