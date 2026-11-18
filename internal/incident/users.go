package incident

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// MaxCallers is the most calling workloads Users names.
const MaxCallers = 5

// Whether users reach what an incident affects (Users.State).
const (
	UsersAffected  = "affected"
	UsersServed    = "served"
	UsersNoRoute   = "no-route"
	UsersNoService = "no-service"
	// UsersUnknown: a route leads to a Service whose endpoints kwatch
	// does not see, so it cannot say.
	UsersUnknown = "unknown"
)

// Users says whether the failing workloads serve user traffic and how
// much is left to serve it, from the routes, Services and endpoints the
// model holds now. It states only what kwatch sees: without a route it
// says it sees none, never that the workload is internal. Nothing
// decides an alert level from it.
type Users struct {
	Text  string
	State string
	// Route is the address users reach, "host/path", and Service the
	// Service behind it, or the Service in front of the failing pods
	// when no route leads to one.
	Route   string `json:",omitempty"`
	Service string `json:",omitempty"`
	// Ready and Total count the Service's endpoints.
	Ready int `json:",omitempty"`
	Total int `json:",omitempty"`
	// Callers are the other workloads whose pods call the Service, by
	// name, at most MaxCallers; MoreCallers counts the rest.
	Callers     []string `json:",omitempty"`
	MoreCallers int      `json:",omitempty"`
}

// usersRoute is one public address and the Service behind it.
type usersRoute struct {
	address string
	service inventory.EntityID
	count   endpoints
}

// UsersOf says whether users are affected by p. nil when model is nil
// or p fails no workload and touches no Service.
func UsersOf(model inventory.Reader, p Incident) *Users {
	if model == nil {
		return nil
	}
	workloads, pods := failingWorkloads(model, p)
	services := usersServices(model, p, pods)
	if len(services) == 0 {
		if len(workloads) == 0 {
			return nil
		}
		return &Users{State: UsersNoService, Text: "kwatch does not " +
			"see a Service or a route in front of " +
			pronoun(len(workloads)) + "."}
	}
	if r, ok := worstRoute(model, services); ok {
		return routedUsers(r)
	}
	return unroutedUsers(model, services, workloads)
}

// pronoun is "it" for one workload, "them" for several.
func pronoun(n int) string {
	if n > 1 {
		return "them"
	}
	return "it"
}

// failingWorkloads are the top owners of p's failing pods that are
// Deployments, StatefulSets or DaemonSets, and those pods.
func failingWorkloads(model inventory.Reader, p Incident) (
	map[inventory.EntityID]bool, []inventory.EntityID,
) {
	workloads := map[inventory.EntityID]bool{}
	if IsWorkload(p.Root.Kind) {
		workloads[p.Root] = true
	}
	seen := map[inventory.EntityID]bool{}
	var pods []inventory.EntityID
	for _, f := range p.Members {
		if !f.Advisory && IsWorkload(f.Entity.Kind) {
			workloads[f.Entity] = true
		}
		pod, ok := rootcause.PodOf(model, f.Entity)
		if f.Advisory || !ok || seen[pod] {
			continue
		}
		seen[pod] = true
		pods = append(pods, pod)
		if owner := rootcause.TopOwner(model, pod); IsWorkload(owner.Kind) {
			workloads[owner] = true
		}
	}
	return workloads, pods
}

// usersServices are the Services among p's objects and those in front of
// its failing pods, sorted.
func usersServices(
	model inventory.Reader, p Incident, pods []inventory.EntityID,
) []inventory.EntityID {
	set := map[inventory.EntityID]bool{}
	for _, id := range append([]inventory.EntityID{p.Root}, p.Impact...) {
		if id.Kind == kube.KindService && id.Name != "" {
			set[id] = true
		}
	}
	for _, pod := range pods {
		for _, id := range model.Related(pod, inventory.Selects,
			inventory.Incoming) {
			if id.Kind == kube.KindService {
				set[id] = true
			}
		}
	}
	return sortedIDs(set)
}

// worstRoute is the public address that leads to one of services with
// the fewest ready endpoints, the more specific path first.
func worstRoute(
	model inventory.Reader, services []inventory.EntityID,
) (usersRoute, bool) {
	var out []usersRoute
	for _, ns := range namespacesOf(services) {
		for _, a := range kube.PublicAddresses(model, ns) {
			svc := inventory.CoreID(kube.KindService, ns, a.Service)
			address := routeAddress(a.Host, a.Path)
			if address == "" || !slices.Contains(services, svc) {
				continue
			}
			out = append(out, usersRoute{address: address, service: svc,
				count: serviceEndpoints(model, svc)})
		}
	}
	if len(out) == 0 {
		return usersRoute{}, false
	}
	slices.SortFunc(out, func(a, b usersRoute) int {
		return cmp.Or(cmp.Compare(a.count.rank(), b.count.rank()),
			cmp.Compare(len(b.address), len(a.address)),
			strings.Compare(a.address, b.address))
	})
	return out[0], true
}

// namespacesOf are the distinct namespaces of ids, sorted.
func namespacesOf(ids []inventory.EntityID) []string {
	var out []string
	for _, id := range ids {
		if !slices.Contains(out, id.Namespace) {
			out = append(out, id.Namespace)
		}
	}
	slices.Sort(out)
	return out
}

// routeAddress is "host/path", the bare host for the root path, or ""
// for a host that is not one.
func routeAddress(host, path string) string {
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, "@?#/ \t") {
		return ""
	}
	path, _, _ = strings.Cut(path, "?")
	path, _, _ = strings.Cut(path, "#")
	if path == "/" || path == "" {
		return host
	}
	return host + strings.TrimRight(path, "/")
}

// endpoints counts a Service's endpoints. seen is false when kwatch
// does not see the Service, known false when it sees no EndpointSlice
// for it nor any in its namespace, so the source may be missing.
type endpoints struct {
	ready, total int
	seen, known  bool
}

// rank orders Services worst first: no ready endpoint, then fewer.
func (e endpoints) rank() int {
	if !e.seen || !e.known {
		return -1
	}
	return e.ready
}

// serviceEndpoints sums the ready and the non-terminating endpoints of
// the Service's slices.
func serviceEndpoints(
	model inventory.Reader, svc inventory.EntityID,
) endpoints {
	out := endpoints{seen: model.Exists(svc)}
	for _, id := range model.Related(svc, inventory.Backs,
		inventory.Incoming) {
		e, ok := model.Entity(id)
		if !ok || id.Kind != kube.KindEndpointSlice {
			continue
		}
		out.known = true
		out.ready += int(attrNumber(e, kube.AttrEndpointsReady))
		out.total += int(attrNumber(e, kube.AttrEndpoints) -
			attrNumber(e, kube.AttrEndpointsTerminating))
	}
	if !out.known && out.seen {
		out.known = len(model.EntitiesIn(kube.KindEndpointSlice,
			svc.Namespace)) > 0
	}
	return out
}

// words says what the endpoints are: "has 2 of 3 ready endpoints",
// "has no endpoints", or what kwatch does not see. ok is false when
// kwatch cannot count them.
func (e endpoints) words() (string, bool) {
	switch {
	case !e.seen:
		return "is a Service kwatch does not see", false
	case !e.known:
		return "has endpoints kwatch does not see", false
	case e.total <= 0:
		return "has no endpoints", true
	}
	return "has " + strconv.Itoa(e.ready) + " of " +
		strconv.Itoa(e.total) + " ready endpoints", true
}
