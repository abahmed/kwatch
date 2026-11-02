package dashboard

import (
	"cmp"
	"slices"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Traffic bounds: the rows kept, the Services one Ingress names.
const (
	maxTraffic  = 8
	maxBackends = 5
)

// endpointCount is how many endpoints a Service has and how many are
// ready; known is false when kwatch cannot tell (no EndpointSlice is
// known in its namespace, so the source may be missing).
type endpointCount struct {
	ready, total int
	known        bool
}

// trafficOf lists the Services and Ingresses p affects with their
// ready endpoints now: those among p's objects, the Services that
// select or route to p's pods, and the Ingresses routing to those.
// Rows with endpoints not ready lead. cut counts the rows left out.
func trafficOf(
	model inventory.Reader, p incident.Incident,
) ([]TrafficRow, int) {
	services, ingresses := trafficObjects(model, p)
	for _, svc := range services {
		for _, id := range model.Related(svc, inventory.RoutesTo,
			inventory.Incoming) {
			if id.Kind == kube.KindIngress && !slices.Contains(ingresses, id) {
				ingresses = append(ingresses, id)
			}
		}
	}
	var out []TrafficRow
	for _, svc := range services {
		if c := endpointsOf(model, svc); c.known {
			out = append(out, TrafficRow{Kind: string(svc.Kind),
				Namespace: svc.Namespace, Name: svc.Name,
				Ready: c.ready, Total: c.total})
		}
	}
	for _, ing := range ingresses {
		if row, ok := ingressRow(model, ing); ok {
			out = append(out, row)
		}
	}
	slices.SortFunc(out, compareTraffic)
	if len(out) > maxTraffic {
		return out[:maxTraffic], len(out) - maxTraffic
	}
	return out, 0
}

// compareTraffic puts rows with endpoints not ready first, then
// Services before Ingresses, then by namespace and name.
func compareTraffic(a, b TrafficRow) int {
	healthy := func(r TrafficRow) int {
		if r.Total > 0 && r.Ready == r.Total {
			return 1
		}
		return 0
	}
	return cmp.Or(cmp.Compare(healthy(a), healthy(b)),
		cmp.Compare(b.Kind, a.Kind), cmp.Compare(a.Namespace, b.Namespace),
		cmp.Compare(a.Name, b.Name))
}

// trafficObjects are the Services and Ingresses among p's objects and
// the Services in front of p's pods, each once.
func trafficObjects(
	model inventory.Reader, p incident.Incident,
) (services, ingresses []inventory.EntityID) {
	add := func(id inventory.EntityID) {
		switch {
		case id.Name == "":
		case id.Kind == kube.KindService && !slices.Contains(services, id):
			services = append(services, id)
		case id.Kind == kube.KindIngress && !slices.Contains(ingresses, id):
			ingresses = append(ingresses, id)
		}
	}
	ids := append([]inventory.EntityID{p.Root}, p.Merged...)
	ids = append(ids, memberIDs(p)...)
	for _, id := range append(ids, p.Impact...) {
		add(id)
	}
	for _, pod := range podsOf(model, p) {
		for _, id := range model.Related(pod, inventory.Selects,
			inventory.Incoming) {
			add(id)
		}
		for _, slice := range model.Related(pod, inventory.RoutesTo,
			inventory.Incoming) {
			for _, id := range model.Related(slice, inventory.Backs,
				inventory.Outgoing) {
				add(id)
			}
		}
	}
	return services, ingresses
}

// endpointsOf adds up the endpoints of svc's EndpointSlices. An
// ExternalName Service has none to count.
func endpointsOf(
	model inventory.Reader, svc inventory.EntityID,
) endpointCount {
	if a, ok := model.Attribute(svc, kube.AttrExternalName); ok &&
		a.Value.AsText() != "" {
		return endpointCount{}
	}
	var c endpointCount
	for _, id := range model.Related(svc, inventory.Backs,
		inventory.Incoming) {
		e, ok := model.Entity(id)
		if !ok || id.Kind != kube.KindEndpointSlice {
			continue
		}
		c.known = true
		c.total += int(number(e, kube.AttrEndpoints))
		c.ready += int(number(e, kube.AttrEndpointsReady))
	}
	if !c.known && model.Exists(svc) {
		// No slice backs it: zero endpoints, when kwatch sees slices
		// at all in its namespace.
		c.known = len(model.EntitiesIn(kube.KindEndpointSlice,
			svc.Namespace)) > 0
	}
	return c
}

// ingressRow adds up the endpoints of the Services ing routes to.
func ingressRow(
	model inventory.Reader, ing inventory.EntityID,
) (TrafficRow, bool) {
	row := TrafficRow{Kind: string(ing.Kind), Namespace: ing.Namespace,
		Name: ing.Name}
	backends := model.Related(ing, inventory.RoutesTo, inventory.Outgoing)
	slices.SortFunc(backends, compareEntities)
	known := false
	for _, svc := range backends {
		if svc.Kind != kube.KindService {
			continue
		}
		c := endpointsOf(model, svc)
		if !c.known {
			continue
		}
		known = true
		row.Ready += c.ready
		row.Total += c.total
		if len(row.Backends) < maxBackends {
			row.Backends = append(row.Backends, svc.Name)
		}
	}
	return row, known
}
