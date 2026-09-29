package filter

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

// underMaintenance reports whether the signal's object, its owning pod, or
// its namespace is deliberately under maintenance.
func (s *Scope) underMaintenance(
	model knowledge.Reader, id knowledge.EntityID,
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

func maintenanceCandidates(id knowledge.EntityID) []knowledge.EntityID {
	out := []knowledge.EntityID{id}
	if id.Kind == kube.KindContainer {
		pod, _, _ := strings.Cut(id.Name, "/")
		out = append(out, knowledge.NewEntityID(
			kube.KindPod, id.Namespace, pod))
	}
	if id.Namespace != "" {
		out = append(out, knowledge.NewEntityID(
			kube.KindNamespace, "", id.Namespace))
	}
	return out
}

func entityHeld(entity knowledge.Entity, now time.Time) bool {
	if attr, ok := entity.Attribute(kube.AttrMaintenance); ok {
		value := strings.TrimSpace(attr.Value.AsText())
		if value != "" && !strings.EqualFold(value, "false") {
			return true
		}
	}
	if attr, ok := entity.Attribute(kube.AttrMaintenanceUntil); ok {
		return attr.Value.AsTime().After(now)
	}
	return false
}
