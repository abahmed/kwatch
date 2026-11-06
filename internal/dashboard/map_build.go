package dashboard

import (
	"slices"
	"strconv"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxOwnerHops bounds the owner walk that folds a pod into its workload
// (pod, ReplicaSet, Deployment; Job, CronJob) and a container into its
// pod's.
const maxOwnerHops = 4

// mapWorkloadKinds are the controllers a namespace's map lists even
// without pods (a Deployment scaled to 0 is still part of the picture).
var mapWorkloadKinds = []inventory.Kind{kube.KindDeployment,
	kube.KindStatefulSet, kube.KindDaemonSet, kube.KindCronJob}

// mapRouteKinds are the objects that route traffic to Services.
var mapRouteKinds = []inventory.Kind{kube.KindIngress, kube.KindHTTPRoute}

// mapLevel is the worst open issue touching an object.
type mapLevel struct {
	level, issue string
}

// mapBuilder reads the model once per request.
type mapBuilder struct {
	model inventory.Reader
	scope Scope
	nodes bool
	worst map[inventory.EntityID]mapLevel
}

func newMapBuilder(
	model inventory.Reader, scope Scope, nodes bool,
) *mapBuilder {
	return &mapBuilder{model: model, scope: scope, nodes: nodes,
		worst: map[inventory.EntityID]mapLevel{}}
}

// markLevels marks each object an open issue touches (its root, members
// and impact, folded into their workloads) with the worst such issue.
// Quiet and resolved issues leave objects ok.
func (b *mapBuilder) markLevels(items []incident.Incident) {
	for _, p := range items {
		if p.State == incident.Resolved || p.Tier <= incident.Silent {
			continue
		}
		touched := []inventory.EntityID{p.Root}
		for key := range p.Members {
			touched = append(touched, key.Entity)
		}
		touched = append(touched, p.Impact...)
		level := p.Tier.String()
		for _, id := range touched {
			id = b.fold(id)
			if old, ok := b.worst[id]; ok &&
				tierRank(old.level) >= tierRank(level) {
				continue
			}
			b.worst[id] = mapLevel{level: level, issue: p.ID}
		}
	}
}

// fold is the object a map draws for id: its topmost owner (a pod's
// Deployment through its ReplicaSet, a Job's CronJob), or id itself.
func (b *mapBuilder) fold(id inventory.EntityID) inventory.EntityID {
	for range maxOwnerHops {
		owners := b.model.Related(id, inventory.OwnedBy,
			inventory.Outgoing)
		if len(owners) == 0 {
			// A container is part of its pod, not owned by it.
			owners = b.model.Related(id, inventory.PartOf,
				inventory.Outgoing)
		}
		if len(owners) == 0 {
			return id
		}
		id = owners[0]
	}
	return id
}

// graph collects a map's nodes and edges without duplicates.
type graph struct {
	b     *mapBuilder
	ns    string
	nodes map[string]MapNode
	edges map[MapEdge]bool
}

func (b *mapBuilder) newGraph(ns string) *graph {
	return &graph{b: b, ns: ns, nodes: map[string]MapNode{},
		edges: map[MapEdge]bool{}}
}

// add draws id; false when its namespace is out of scope.
func (g *graph) add(id inventory.EntityID) (string, bool) {
	if !mapScope(g.b.scope, id.Namespace) {
		return "", false
	}
	key := nodeID(id)
	if _, ok := g.nodes[key]; ok {
		return key, true
	}
	n := MapNode{ID: key, Kind: string(id.Kind), Name: id.Name,
		Namespace: id.Namespace, Level: mapLevelOK,
		Ready:    g.b.ready(id),
		External: g.ns != "" && id.Namespace != "" && id.Namespace != g.ns}
	if l, ok := g.b.worst[id]; ok {
		n.Level, n.Issue = l.level, l.issue
	}
	g.nodes[key] = n
	return key, true
}

// link draws from and to, and the edge between them.
func (g *graph) link(from, to inventory.EntityID, kind string) {
	a, okA := g.add(from)
	z, okZ := g.add(to)
	if okA && okZ && a != z {
		g.edges[MapEdge{From: a, To: z, Type: kind}] = true
	}
}

// namespace draws one namespace: its routes, Services, workloads (pods
// folded in) and what connects them.
func (b *mapBuilder) namespace(ns string) *graph {
	g := b.newGraph(ns)
	for _, kind := range mapWorkloadKinds {
		for _, id := range b.model.EntitiesIn(kind, ns) {
			g.add(b.fold(id))
		}
	}
	for _, kind := range mapRouteKinds {
		for _, route := range b.model.EntitiesIn(kind, ns) {
			g.add(route)
			for _, to := range b.model.Related(route, inventory.RoutesTo,
				inventory.Outgoing) {
				if to.Kind == kube.KindService {
					g.link(route, to, "routes")
				}
			}
		}
	}
	for _, svc := range b.model.EntitiesIn(kube.KindService, ns) {
		g.add(svc)
		for _, pod := range b.model.Related(svc, inventory.Selects,
			inventory.Outgoing) {
			g.link(svc, b.fold(pod), "selects")
		}
	}
	for _, pod := range b.model.EntitiesIn(kube.KindPod, ns) {
		w := b.fold(pod)
		g.add(w)
		for _, to := range b.model.Related(pod, inventory.Calls,
			inventory.Outgoing) {
			g.link(w, b.fold(to), "calls")
		}
		if b.nodes {
			for _, node := range b.model.Related(pod, inventory.RunsOn,
				inventory.Outgoing) {
				g.link(w, node, "runs-on")
			}
		}
	}
	return g
}

// ready is a workload's ready replicas of those it wants, "2/3".
func (b *mapBuilder) ready(id inventory.EntityID) string {
	want := kube.AttrReplicas
	switch id.Kind {
	case kube.KindDaemonSet:
		want = kube.AttrDesiredReplicas
	case kube.KindDeployment, kube.KindStatefulSet, kube.KindReplicaSet:
	default:
		return ""
	}
	desired, ok := b.number(id, want)
	if !ok {
		return ""
	}
	ready, _ := b.number(id, kube.AttrReadyReplicas)
	return strconv.Itoa(int(ready)) + "/" + strconv.Itoa(int(desired))
}

func (b *mapBuilder) number(id inventory.EntityID, name string) (
	float64, bool,
) {
	a, ok := b.model.Attribute(id, name)
	if !ok {
		return 0, false
	}
	return a.Value.AsNumber()
}

// namespaces are the watched namespaces the model holds, sorted.
func (b *mapBuilder) namespaces() []string {
	var out []string
	for _, id := range b.model.Entities(kube.KindNamespace) {
		if mapScope(b.scope, id.Name) {
			out = append(out, id.Name)
		}
	}
	slices.Sort(out)
	return out
}

// failingIn counts the failing objects of each namespace.
func (b *mapBuilder) failingIn() map[string]int {
	out := map[string]int{}
	for id := range b.worst {
		if id.Namespace != "" && mapScope(b.scope, id.Namespace) {
			out[id.Namespace]++
		}
	}
	return out
}

// build answers one request.
func (b *mapBuilder) build(view, ns string) MapResponse {
	out := MapResponse{View: view, Namespaces: b.namespaces(),
		Nodes: []MapNode{}, Edges: []MapEdge{}}
	if out.Namespaces == nil {
		out.Namespaces = []string{}
	}
	var g *graph
	if view == mapViewFailing {
		g = b.failing()
	} else {
		ns = b.pick(ns, out.Namespaces)
		out.Namespace = ns
		if ns == "" || !mapScope(b.scope, ns) {
			return out
		}
		g = b.namespace(ns)
	}
	out.Count = len(g.nodes)
	if out.Count > maxMapNodes {
		out.TooMany = true
		return out
	}
	for _, n := range g.nodes {
		out.Nodes = append(out.Nodes, n)
	}
	for e := range g.edges {
		out.Edges = append(out.Edges, e)
	}
	sortMap(&out)
	return out
}

// pick is the namespace to draw: the one asked for, else the one with
// the most failing objects, else the first.
func (b *mapBuilder) pick(ns string, all []string) string {
	if ns != "" || len(all) == 0 {
		return ns
	}
	best, most := all[0], 0
	for name, n := range b.failingIn() {
		if n > most || (n == most && most > 0 && name < best) {
			best, most = name, n
		}
	}
	return best
}

// failing draws, across the watched namespaces, every failing object
// and the objects one connection away from it.
func (b *mapBuilder) failing() *graph {
	out := b.newGraph("")
	names := make([]string, 0, len(b.failingIn()))
	for ns := range b.failingIn() {
		names = append(names, ns)
	}
	slices.Sort(names)
	for _, ns := range names {
		g := b.namespace(ns)
		keep := map[string]bool{}
		for key, n := range g.nodes {
			if n.Level != mapLevelOK {
				keep[key] = true
			}
		}
		for e := range g.edges {
			if keep[e.From] || keep[e.To] {
				out.edges[e] = true
				out.nodes[e.From], out.nodes[e.To] =
					g.nodes[e.From], g.nodes[e.To]
			}
		}
		for key := range keep {
			out.nodes[key] = g.nodes[key]
		}
	}
	for key, n := range out.nodes {
		n.External = false
		out.nodes[key] = n
	}
	return out
}
