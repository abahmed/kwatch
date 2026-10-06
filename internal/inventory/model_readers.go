package inventory

// referrerRelations point at a Service from the objects judged by its
// endpoints: a webhook, API service or conversion webhook it serves, and
// an Ingress that routes to it.
var referrerRelations = []RelationType{Serves, RoutesTo}

// backedReferrers returns the entities that read the Services id backs
// (an EndpointSlice backs its Service). When the slice changes, the
// Service's endpoints change, and so must the verdict on those who depend
// on the Service: without this they would only be judged again when they
// are themselves observed. The caller holds the write lock.
func (m *Model) backedReferrers(id EntityID) []EntityID {
	var out []EntityID
	for _, service := range m.edges.neighbors(id, Backs, Outgoing) {
		for _, relation := range referrerRelations {
			out = append(out,
				m.edges.neighbors(service, relation, Incoming)...)
		}
	}
	return out
}
