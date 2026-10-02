package scope

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// underMaintenance reports whether the finding's object, its owning pod, or
// its namespace is deliberately under maintenance.
func (s *Scope) underMaintenance(
	model inventory.Reader, id inventory.EntityID,
) bool {
	if !s.maintenance || s.now == nil || model == nil {
		return false
	}
	now := s.now()
	for _, candidate := range maintenanceCandidates(id) {
		entity, ok := model.Entity(candidate)
		if ok && entityHeld(entity, now) {
			return true
		}
	}
	return false
}

func maintenanceCandidates(id inventory.EntityID) []inventory.EntityID {
	out := []inventory.EntityID{id}
	if id.Kind == kube.KindContainer {
		pod, _, _ := strings.Cut(id.Name, "/")
		out = append(out, inventory.CoreID(
			kube.KindPod, id.Namespace, pod))
	}
	if id.Namespace != "" {
		out = append(out, inventory.CoreID(
			kube.KindNamespace, "", id.Namespace))
	}
	return out
}

// entityHeld reports whether an entity is held. The annotation holds only
// with true, 1, yes or on (case-insensitive); any other value, including
// false, does not. A valid until time bounds the hold whether or not the
// annotation is set: once it has passed the entity is no longer held. An
// explicitly non-true annotation wins over the until time.
func entityHeld(entity inventory.Entity, now time.Time) bool {
	untilAttr, hasUntil := entity.Attribute(kube.AttrMaintenanceUntil)
	live := hasUntil && untilAttr.Value.AsTime().After(now)
	attr, hasAnnotation := entity.Attribute(kube.AttrMaintenance)
	if !hasAnnotation {
		return live
	}
	if !maintenanceTruthy(attr.Value.AsText()) {
		return false
	}
	return !hasUntil || live
}

func maintenanceTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}
