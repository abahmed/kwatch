package explain

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// objectPlace builds a place in an object's own spec.
func objectPlace(
	object inventory.EntityID, path, value string,
) *detection.Place {
	return &detection.Place{Object: object, Path: path, Value: value}
}

// hpaMaxPlace is the ceiling of an autoscaler that cannot add more.
func (s Snapshot) hpaMaxPlace(c Cause) *detection.Place {
	hpa, ok := s.Model.Entity(c.Root)
	if !ok {
		return nil
	}
	limit, ok := attrNumber(hpa, kube.AttrMaxReplicas)
	if !ok {
		return nil
	}
	return objectPlace(c.Root, "spec.maxReplicas",
		strconv.FormatFloat(limit, 'f', -1, 64))
}

// budgetPlace is the setting of a disruption budget that blocks a
// drain: the budget sets either minAvailable or maxUnavailable.
func (s Snapshot) budgetPlace(c Cause) *detection.Place {
	pdb, ok := s.Model.Entity(c.Root)
	if !ok {
		return nil
	}
	for _, field := range []struct{ attr, path string }{
		{kube.AttrMinAvailable, "spec.minAvailable"},
		{kube.AttrMaxUnavailable, "spec.maxUnavailable"},
	} {
		if value := attrText(pdb, field.attr); value != "" {
			return objectPlace(c.Root, field.path, value)
		}
	}
	return nil
}

// routeBackendPlace is the backend a route names but that does not
// exist: the Ingress (or route) that points at it, and the name it
// points at.
func (s Snapshot) routeBackendPlace(c Cause) *detection.Place {
	for _, id := range c.Covers {
		switch id.Kind {
		case kube.KindIngress:
			return objectPlace(id,
				"spec.rules[].http.paths[].backend.service.name",
				c.Root.Name)
		case kube.KindPod, kube.KindContainer:
		default:
			return objectPlace(id, "spec.rules[].backendRefs[].name",
				c.Root.Name)
		}
	}
	return nil
}

// claimClassPlace is the storage class a claim names but the cluster
// does not have.
func (s Snapshot) claimClassPlace(c Cause) *detection.Place {
	for _, id := range c.Covers {
		if id.Kind == kube.KindPVC {
			return objectPlace(id, "spec.storageClassName", c.Root.Name)
		}
	}
	return nil
}

// claimSizePlace is the size of a claim whose volume is full. It says
// nothing for a full inode table, which a bigger volume may not fix, or
// a claim that failed for another reason.
func (s Snapshot) claimSizePlace(c Cause) *detection.Place {
	if c.Mode != detection.ModeVolumeFull {
		return nil
	}
	claim, ok := s.Model.Entity(c.Root)
	if !ok || c.Root.Kind != kube.KindPVC {
		return nil
	}
	size, ok := attrNumber(claim, kube.AttrRequested)
	if !ok || size <= 0 {
		return nil
	}
	return objectPlace(c.Root, "spec.resources.requests.storage",
		detection.MemoryText(size))
}
