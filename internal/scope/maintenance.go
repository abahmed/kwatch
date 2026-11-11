package scope

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// underMaintenance reports whether the finding's object, its pod, an
// owner of either (a ReplicaSet and its Deployment, a Job and its
// CronJob), or its namespace is deliberately under maintenance.
func (s *Scope) underMaintenance(
	model inventory.Reader, id inventory.EntityID,
) bool {
	if !s.maintenance || s.now == nil || model == nil {
		return false
	}
	now := s.now()
	for _, candidate := range maintenanceCandidates(model, id) {
		entity, ok := model.Entity(candidate)
		if ok && entityHeld(entity, now) {
			return true
		}
	}
	return false
}

func maintenanceCandidates(
	model inventory.Reader, id inventory.EntityID,
) []inventory.EntityID {
	out := []inventory.EntityID{id}
	start := id
	if id.Kind == kube.KindContainer {
		pod, _, _ := strings.Cut(id.Name, "/")
		start = inventory.CoreID(kube.KindPod, id.Namespace, pod)
		out = append(out, start)
	}
	// The owners up to the top-level workload: "annotate an object"
	// holds everything it runs. That includes an owner without a
	// namespace: the Node of a static pod, or a cluster-scoped custom
	// resource (inventory.Owner finds it).
	for hop := 0; hop < maxMaintenanceOwners; hop++ {
		owner, ok := inventory.Owner(model, start)
		if !ok {
			break
		}
		start = owner
		out = append(out, start)
	}
	if id.Namespace != "" {
		out = append(out, inventory.CoreID(
			kube.KindNamespace, "", id.Namespace))
	}
	return out
}

// maxMaintenanceOwners bounds the owner chain followed: a pod's
// ReplicaSet and Deployment, or a Job's CronJob, with room to spare.
const maxMaintenanceOwners = 4

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
