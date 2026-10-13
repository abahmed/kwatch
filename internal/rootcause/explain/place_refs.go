package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// missingRefPlace is where an object that does not exist is asked for:
// the field of a container (or pod volume) that names the missing
// Secret or ConfigMap, or the Ingress that names a missing TLS Secret.
// The Value is the object's name: never a value of the Secret.
func (s Snapshot) missingRefPlace(c Cause) *detection.Place {
	if c.Mode != ModeMissing {
		return nil
	}
	name := string(c.Root.Kind) + " " + c.Root.Name
	for _, id := range c.Covers {
		if id.Kind == kube.KindIngress && c.Root.Kind == kube.KindSecret {
			return objectPlace(id, "spec.tls[].secretName", c.Root.Name)
		}
	}
	key := kube.RefAttr(c.Root.Kind, c.Root.Name)
	for _, container := range s.containersOf(c) {
		if path := attrText(container, key); path != "" {
			return s.containerPlace(container, path, name)
		}
	}
	for _, id := range placeEntities(c) {
		for _, pod := range s.podsOf(id) {
			if place := s.podRefPlace(pod, key, name); place != nil {
				return place
			}
		}
	}
	return nil
}

// podRefPlace names the pod volume that refers to the object.
func (s Snapshot) podRefPlace(
	pod inventory.EntityID, key, name string,
) *detection.Place {
	entity, ok := s.Model.Entity(pod)
	if !ok {
		return nil
	}
	path := attrText(entity, key)
	if path == "" {
		return nil
	}
	return &detection.Place{Object: s.topOwner(pod), Path: path,
		Value: name}
}
