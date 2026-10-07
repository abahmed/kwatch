package incident

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxOwnerDepth bounds the walk up the owners of the root.
const maxOwnerDepth = 5

// ownerOf is who an incident belongs to, for routing: the owner of its
// root, else of the workload that owns the root, else of the root's
// namespace. It is empty when none has one, and the incident then goes to
// the routes that ask for no owner.
func (m *Manager) ownerOf(p *Incident) string {
	if m.model == nil {
		return ""
	}
	id := p.Root
	for depth := 0; depth < maxOwnerDepth; depth++ {
		if owner := m.ownerAttribute(id); owner != "" {
			return owner
		}
		owners := m.model.Related(id, inventory.OwnedBy, inventory.Outgoing)
		if len(owners) == 0 {
			break
		}
		id = owners[0]
	}
	if p.Root.Namespace == "" {
		return ""
	}
	return m.ownerAttribute(
		inventory.CoreID(kube.KindNamespace, "", p.Root.Namespace))
}

func (m *Manager) ownerAttribute(id inventory.EntityID) string {
	entity, ok := m.model.Entity(id)
	if !ok {
		return ""
	}
	if attr, ok := entity.Attribute(kube.AttrOwner); ok {
		return attr.Value.AsText()
	}
	return ""
}

// ownersOf is the owner as the list a route carries.
func ownersOf(owner string) []string {
	if owner == "" {
		return nil
	}
	return []string{owner}
}
