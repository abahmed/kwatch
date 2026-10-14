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
	return m.ownerOfEntity(p.Root)
}

// ownerOfEntity is the owner of one entity by the same walk.
func (m *Manager) ownerOfEntity(root inventory.EntityID) string {
	id := root
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
	if root.Namespace == "" {
		return ""
	}
	return m.ownerAttribute(
		inventory.CoreID(kube.KindNamespace, "", root.Namespace))
}

// reachOf is who a cluster-scoped incident (a node, a zone) touches, for
// routing: the owners and namespaces of its members and of the pods on
// its nodes. A node has no owner and no namespace, and its pods fail
// minutes after it does, so without this the first message would reach
// no team. An incident with a namespaced root keeps the root's owner.
func (m *Manager) reachOf(p *Incident) (owners, namespaces []string) {
	if m.model == nil || p.Root.Namespace != "" {
		return ownersOf(p.Owner), nil
	}
	owned, spaces := map[string]bool{}, map[string]bool{}
	visit := func(id inventory.EntityID) {
		if id.Namespace != "" {
			spaces[id.Namespace] = true
		}
		if owner := m.ownerOfEntity(id); owner != "" {
			owned[owner] = true
		}
	}
	nodes := []inventory.EntityID{p.Root}
	for _, f := range p.Members {
		visit(f.Entity)
		nodes = append(nodes, f.Entity)
	}
	for _, node := range nodes {
		if node.Kind != kube.KindNode {
			continue
		}
		for _, pod := range m.model.Related(node, inventory.RunsOn,
			inventory.Incoming) {
			visit(pod)
		}
	}
	return sortedKeys(owned), sortedKeys(spaces)
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
