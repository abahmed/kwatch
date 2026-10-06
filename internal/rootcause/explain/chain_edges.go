package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A chain step joins two failing anchors. An anchor is the unit that
// fails as one: a workload stands for its pods and containers, any
// other entity stands for itself. Three kinds of step exist, and each
// needs its own evidence:
//
//   - calls: a pod that is not ready, or that fails on a call, calls a
//     Service that has no ready endpoint (its error names the Service, or
//     its configuration does), and the failing anchors behind that
//     Service are the cause. The effect must begin strictly after.
//   - backs: the failing pods behind a Service that has no ready
//     endpoint are the cause of the Service. The Service may begin at
//     the same time as its pods: it only restates them.
//   - routes-to: an Ingress or route that sends traffic to a failing
//     Service that has no ready endpoint.
//
// Both ends must fail, so a chain never passes through a healthy
// entity, and both must have a start time in the right order, within
// the causal window.

// chainEdge is one step, from the anchor that began first to the one
// that follows from it.
type chainEdge struct {
	from, to inventory.EntityID
	link     LinkType
}

// chainGraph holds the failing anchors of a solve and the steps
// between them.
type chainGraph struct {
	// members lists the failing entities of each anchor, sorted.
	members map[inventory.EntityID][]inventory.EntityID
	// anchor maps a failing entity to its anchor.
	anchor map[inventory.EntityID]inventory.EntityID
	// began is the earliest start of an anchor's failures.
	began map[inventory.EntityID]time.Time
	// out lists the steps leaving an anchor, in is the reverse.
	out, in map[inventory.EntityID][]chainEdge
	// services are the Services read while building the steps into
	// an anchor: when one changes, the steps may change.
	services map[inventory.EntityID][]inventory.EntityID
}

func (g *chainGraph) empty() bool { return len(g.out) == 0 }

// has reports a failing anchor, with or without findings yet.
func (g *chainGraph) has(anchor inventory.EntityID) bool {
	_, ok := g.members[anchor]
	return ok
}

// anchorOf is the unit that fails as one: a pod and its containers
// belong to their top owner, anything else is its own anchor.
func (v *view) anchorOf(id inventory.EntityID) inventory.EntityID {
	unit := id
	if pod, ok := v.podOf(id); ok {
		unit = pod
	}
	return rootcause.TopOwner(v.s.Model, unit)
}

// chainGraph builds the steps between the failing anchors of failing.
func (v *view) chainGraph(failing []inventory.EntityID) *chainGraph {
	g := &chainGraph{
		members:  map[inventory.EntityID][]inventory.EntityID{},
		anchor:   map[inventory.EntityID]inventory.EntityID{},
		began:    map[inventory.EntityID]time.Time{},
		out:      map[inventory.EntityID][]chainEdge{},
		in:       map[inventory.EntityID][]chainEdge{},
		services: map[inventory.EntityID][]inventory.EntityID{},
	}
	for _, failure := range failing {
		a := v.anchorOf(failure)
		g.members[a] = append(g.members[a], failure)
		g.anchor[failure] = a
	}
	for a, members := range g.members {
		if t := v.earliestSince(members); t.ok {
			g.began[a] = t.t
		}
	}
	v.addUnreadyBackends(g)
	for _, a := range sortedKeys(g.members) {
		v.callSteps(g, a)
		v.backsSteps(g, a)
		v.routeSteps(g, a)
	}
	return g
}

// addStep records a step when both ends began in the right order and
// within the causal window. A calls step needs the effect strictly after
// its cause. A step whose effect only restates its cause (a Service its
// pods, a route its Service) may begin with it, give or take the clock
// skew between sources.
func (v *view) addStep(g *chainGraph, e chainEdge) {
	cause, effect := g.began[e.from], g.began[e.to]
	if e.from == e.to || cause.IsZero() || effect.IsZero() ||
		effect.Sub(cause) > v.s.window() {
		return
	}
	if e.link == LinkCalls && !effect.After(cause) ||
		effect.Before(cause.Add(-TemporalSlack)) {
		return
	}
	for _, known := range g.out[e.from] {
		if known.to == e.to {
			return
		}
	}
	g.out[e.from] = append(g.out[e.from], e)
	g.in[e.to] = append(g.in[e.to], e)
}

// callSteps adds the calls steps into anchor a from the Services its
// pods call.
func (v *view) callSteps(g *chainGraph, a inventory.EntityID) {
	for _, unit := range v.callerPods(g, a) {
		for _, service := range v.calledServices(unit) {
			g.services[a] = append(g.services[a], service)
			if !v.noReadyEndpoints(service) {
				continue
			}
			for _, provider := range v.providersOf(g, service) {
				v.addStep(g, chainEdge{from: provider, to: a,
					link: LinkCalls})
			}
		}
	}
}

// callerPods are the pods of anchor a that may be failing on a call: its
// failing pods and, for a workload, its pods that are not ready, which
// show no finding of their own yet when the workload's summary does.
func (v *view) callerPods(
	g *chainGraph, a inventory.EntityID,
) []inventory.EntityID {
	set := map[inventory.EntityID]bool{}
	for _, failure := range g.members[a] {
		if unit, ok := v.unitOf(failure); ok {
			set[unit] = true
		}
	}
	if a.Kind != kube.KindPod {
		for _, pod := range sampleIDs(rootcause.OwnedPods(v.s.Model, a)) {
			set[pod] = true
		}
	}
	var out []inventory.EntityID
	for _, pod := range sortedKeys(set) {
		if v.chainCaller(pod) {
			out = append(out, pod)
		}
	}
	return out
}

// chainCaller reports a pod that is not ready, or that fails, in ways a
// failed call can cause (see chainModes). A pod with another failure,
// such as OOMKilled, has a cause of its own, and a pod that is ready is
// not failing on anything.
func (v *view) chainCaller(pod inventory.EntityID) bool {
	ids := append([]inventory.EntityID{pod}, v.s.Model.Related(
		pod, inventory.PartOf, inventory.Incoming)...)
	failing := false
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			if !unhealthy(f) {
				continue
			}
			if !anyModeMatches(chainModes, f.Mode) {
				return false
			}
			failing = true
		}
	}
	return failing || v.notReady(pod)
}

// notReady reports a pod the model shows as not ready.
func (v *view) notReady(pod inventory.EntityID) bool {
	entity, ok := v.s.Model.Entity(pod)
	if !ok {
		return false
	}
	attribute, ok := entity.Attribute(kube.AttrReady)
	if !ok {
		return false
	}
	ready, _ := attribute.Value.AsBool()
	return !ready
}

// calledServices are the Services a pod calls: the one its error names,
// or else the ones its configuration names that exist.
func (v *view) calledServices(pod inventory.EntityID) []inventory.EntityID {
	if call, ok := v.clusterCallOf(pod); ok {
		return []inventory.EntityID{call.service}
	}
	entity, ok := v.s.Model.Entity(pod)
	if !ok {
		return nil
	}
	var out []inventory.EntityID
	for _, ref := range kube.ServiceCalls(entity) {
		if v.s.Model.Exists(ref.Service) {
			out = append(out, ref.Service)
		}
	}
	if len(out) > maxCallEdges {
		out = out[:maxCallEdges]
	}
	return out
}

// providersOf are the failing anchors behind a Service: the Service
// itself and the workloads that own the pods it selects.
func (v *view) providersOf(
	g *chainGraph, service inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	if g.has(service) {
		out = append(out, service)
	}
	for _, owner := range v.backendOwners(service) {
		if g.has(owner) {
			out = append(out, owner)
		}
	}
	for _, pod := range sortedKeys(v.selected(service)) {
		if a := v.anchorOf(pod); a == pod && g.has(a) {
			out = append(out, a)
		}
	}
	return out
}

// backsSteps adds the steps from the failing pods behind a Service to
// the Service, when anchor a is a Service with no ready endpoint.
func (v *view) backsSteps(g *chainGraph, a inventory.EntityID) {
	if a.Kind != kube.KindService || !v.hasMode(a,
		[]detection.Mode{detection.ModeNoEndpoints}) ||
		!v.noReadyEndpoints(a) {
		return
	}
	g.services[a] = append(g.services[a], a)
	for _, pod := range sortedKeys(v.selected(a)) {
		if from := v.anchorOf(pod); g.has(from) {
			v.addStep(g, chainEdge{from: from, to: a, link: LinkBacks})
		}
	}
}

// routeSteps adds the steps from a failing Service with no ready
// endpoint to the Ingress or route anchor a that sends traffic to it.
func (v *view) routeSteps(g *chainGraph, a inventory.EntityID) {
	for _, service := range v.s.Model.Related(
		a, inventory.RoutesTo, inventory.Outgoing,
	) {
		if g.has(service) && v.noReadyEndpoints(service) {
			g.services[a] = append(g.services[a], service)
			v.addStep(g, chainEdge{from: service, to: a,
				link: LinkRoutesTo})
		}
	}
}

// widen adds to focus the failures chained to it, in either direction,
// so one solve sees a whole chain: a failure explained by something
// upstream is solved with it, and a cause is solved with what follows
// from it. The result is sorted.
func (g *chainGraph) widen(
	focus []inventory.EntityID,
) []inventory.EntityID {
	seen := map[inventory.EntityID]bool{}
	var queue []inventory.EntityID
	visit := func(a inventory.EntityID) {
		if !seen[a] {
			seen[a] = true
			queue = append(queue, a)
		}
	}
	for _, id := range focus {
		if a, ok := g.anchor[id]; ok {
			visit(a)
		}
	}
	for i := 0; i < len(queue); i++ {
		for _, e := range g.out[queue[i]] {
			visit(e.to)
		}
		for _, e := range g.in[queue[i]] {
			visit(e.from)
		}
	}
	out := map[inventory.EntityID]bool{}
	for _, id := range focus {
		out[id] = true
	}
	for _, a := range queue {
		for _, member := range g.members[a] {
			out[member] = true
		}
	}
	return sortedKeys(out)
}

// inputsOf are the Services read for the steps into the anchors of
// failures: their changes may change the steps.
func (g *chainGraph) inputsOf(
	failures []inventory.EntityID,
) []inventory.EntityID {
	set := map[inventory.EntityID]bool{}
	for _, failure := range failures {
		for _, service := range g.services[g.anchor[failure]] {
			set[service] = true
		}
	}
	return sortedKeys(set)
}

// addUnreadyBackends adds the workloads behind a failing Service that
// has no ready endpoint, whose pods are all not ready, as anchors that
// have no finding yet. The detectors wait before they call unready pods
// a failure, but the Service shows the effect at once, and the pods'
// own readiness is evidence of what its endpoints already say.
func (v *view) addUnreadyBackends(g *chainGraph) {
	for _, service := range sortedKeys(g.members) {
		if service.Kind != kube.KindService || !v.hasMode(service,
			[]detection.Mode{detection.ModeNoEndpoints}) ||
			!v.noReadyEndpoints(service) {
			continue
		}
		pods := map[inventory.EntityID][]inventory.EntityID{}
		for _, pod := range sortedKeys(v.selected(service)) {
			owner := v.anchorOf(pod)
			pods[owner] = append(pods[owner], pod)
		}
		for _, owner := range sortedKeys(pods) {
			if _, known := g.members[owner]; known {
				continue
			}
			if since, ok := v.unreadySince(pods[owner]); ok {
				g.members[owner] = nil
				g.began[owner] = since
			}
		}
	}
}

// unreadySince is when the earliest of the pods stopped being ready,
// when none of them is ready.
func (v *view) unreadySince(pods []inventory.EntityID) (time.Time, bool) {
	var first time.Time
	for _, pod := range pods {
		entity, ok := v.s.Model.Entity(pod)
		if !ok || !v.notReady(pod) {
			return time.Time{}, false
		}
		attribute, _ := entity.Attribute(kube.AttrReady)
		if first.IsZero() || attribute.Since.Before(first) {
			first = attribute.Since
		}
	}
	return first, !first.IsZero()
}
