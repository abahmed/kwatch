package inventory

// kindIndex holds the present entities by kind, then by namespace, so a
// query for one namespace does not have to walk the whole cluster.
// Cluster-scoped entities live under the empty namespace.
type kindIndex map[Kind]map[string]map[EntityID]struct{}

func (x kindIndex) add(id EntityID) {
	byNamespace := x[id.Kind]
	if byNamespace == nil {
		byNamespace = make(map[string]map[EntityID]struct{})
		x[id.Kind] = byNamespace
	}
	ids := byNamespace[id.Namespace]
	if ids == nil {
		ids = make(map[EntityID]struct{})
		byNamespace[id.Namespace] = ids
	}
	ids[id] = struct{}{}
}

func (x kindIndex) remove(id EntityID) {
	byNamespace := x[id.Kind]
	delete(byNamespace[id.Namespace], id)
	if len(byNamespace[id.Namespace]) == 0 {
		delete(byNamespace, id.Namespace)
	}
	if len(byNamespace) == 0 {
		delete(x, id.Kind)
	}
}

// EntitiesIn returns the present entities of one kind in one namespace,
// in deterministic order. The empty namespace means cluster-scoped.
func (m *Model) EntitiesIn(kind Kind, namespace string) []EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.byKind[kind][namespace]
	out := make([]EntityID, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sortIDs(out)
	return out
}

// CoreEntitiesNamed returns the present built-in-group entities of one
// kind with one name, in any namespace, in deterministic order. It costs
// one map lookup per namespace rather than a walk over every entity.
func (m *Model) CoreEntitiesNamed(kind Kind, name string) []EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []EntityID
	for namespace, ids := range m.byKind[kind] {
		id := CoreID(kind, namespace, name)
		if _, ok := ids[id]; ok {
			out = append(out, id)
		}
	}
	sortIDs(out)
	return out
}

// AttributeIn returns one attribute of every present entity of a kind in
// a namespace that has it, keyed by entity. It reads under one lock and
// copies only that attribute, which is much cheaper than calling Entity
// for each of hundreds of pods.
func (m *Model) AttributeIn(
	kind Kind, namespace, attribute string,
) map[EntityID]Value {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := m.byKind[kind][namespace]
	out := make(map[EntityID]Value, len(ids))
	for id := range ids {
		if attr, ok := m.records[id].entity.Attributes[attribute]; ok {
			out[id] = attr.Value
		}
	}
	return out
}
