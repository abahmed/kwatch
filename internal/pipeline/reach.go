package pipeline

import (
	"cmp"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withReach adds, to a decision about an incident that reaches an
// Ingress, a route or a Service, the addresses users reach it at. The
// writer reads them as data; it never reads the model.
func (a *announcer) withReach(d incident.Decision) incident.Decision {
	if d.Action == incident.Resolve || a.history == nil {
		return d
	}
	d.Facts.Reach = reachOf(a.history, d.Incident)
	return d
}

// reachOf lists the public addresses of the incident's Ingresses and
// routes, and of any address that leads to an affected Service, worst
// first: no ready endpoint, then the more specific path.
func reachOf(model inventory.Reader, p incident.Incident) []incident.Reach {
	sources := map[inventory.EntityID]bool{}
	services := map[inventory.EntityID]bool{}
	namespaces := map[string]bool{}
	for _, id := range append([]inventory.EntityID{p.Root}, p.Impact...) {
		switch id.Kind {
		case kube.KindIngress, kube.KindHTTPRoute:
			sources[id] = true
		case kube.KindService:
			services[id] = true
		default:
			continue
		}
		namespaces[id.Namespace] = true
	}
	var out []incident.Reach
	seen := map[string]bool{}
	for namespace := range namespaces {
		for _, addr := range kube.PublicAddresses(model, namespace) {
			service := inventory.CoreID(kube.KindService, namespace,
				addr.Service)
			host := cleanHost(addr.Host)
			ready, total := endpointCounts(model, service)
			// An Ingress in the impact leads to its Services; only the
			// addresses whose Service has lost endpoints are told.
			affected := services[service] ||
				(sources[addr.Source] && ready < total)
			if host == "" || !affected {
				continue
			}
			path := cleanPath(addr.Path)
			key := host + path + "\x00" + addr.Source.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, incident.Reach{Host: host, Path: path,
				Source: addr.Source, Ready: ready, Total: total})
		}
	}
	slices.SortFunc(out, compareReach)
	if len(out) > incident.MaxReach {
		out = out[:incident.MaxReach]
	}
	return out
}

// compareReach puts the addresses with no ready endpoint first, then
// the more specific path, then by address.
func compareReach(a, b incident.Reach) int {
	return cmp.Or(
		cmp.Compare(boolRank(b.Down()), boolRank(a.Down())),
		cmp.Compare(len(b.Path), len(a.Path)),
		strings.Compare(a.Address(), b.Address()))
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cleanHost keeps a host as written (a wildcard stays "*.example.com")
// and refuses anything that is not one: credentials, a path or a query
// in a host never reach a message.
func cleanHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, "@?#/ \t") {
		return ""
	}
	return host
}

// cleanPath is "" for the root path and a path without its query.
func cleanPath(path string) string {
	path, _, _ = strings.Cut(path, "?")
	path, _, _ = strings.Cut(path, "#")
	if path == "/" || path == "" {
		return ""
	}
	return strings.TrimRight(path, "/")
}

// endpointCounts sums the ready and the non-terminating endpoints of
// the Service's slices.
func endpointCounts(
	model inventory.Reader, service inventory.EntityID,
) (ready, total int) {
	for _, id := range model.Related(service, inventory.Backs,
		inventory.Incoming) {
		slice, ok := model.Entity(id)
		if !ok {
			continue
		}
		up, _ := numberAttr(slice, kube.AttrEndpointsReady)
		all, _ := numberAttr(slice, kube.AttrEndpoints)
		leaving, _ := numberAttr(slice, kube.AttrEndpointsTerminating)
		ready += int(up)
		total += int(all - leaving)
	}
	return ready, total
}

func numberAttr(e inventory.Entity, name string) (float64, bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return 0, false
	}
	return attribute.Value.AsNumber()
}
