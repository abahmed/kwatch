package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ReachRow is one address users reach a Service or an Ingress at: a
// host and, when the rule has a specific one, a path, with the ready
// and total endpoints behind it.
type ReachRow struct {
	Address string `json:"address"`
	Ready   int    `json:"ready"`
	Total   int    `json:"total"`
}

// reachOf lists the public addresses of a traffic row: those that lead
// to the Service, or that the Ingress publishes. The addresses with no
// ready endpoint come first, then the more specific paths. Hosts with
// credentials, queries or paths in them are left out.
func reachOf(model inventory.Reader, row TrafficRow) []ReachRow {
	var out []ReachRow
	seen := map[string]bool{}
	for _, a := range kube.PublicAddresses(model, row.Namespace) {
		service := inventory.CoreID(kube.KindService, row.Namespace,
			a.Service)
		leads := (row.Kind == string(kube.KindService) &&
			a.Service == row.Name) ||
			(row.Kind == string(kube.KindIngress) &&
				a.Source.Name == row.Name &&
				a.Source.Kind == kube.KindIngress)
		address := addressText(a.Host, a.Path)
		if !leads || address == "" || seen[address] {
			continue
		}
		seen[address] = true
		c := endpointsOf(model, service)
		out = append(out, ReachRow{Address: address, Ready: c.ready,
			Total: c.total})
	}
	slices.SortFunc(out, func(a, b ReachRow) int {
		return cmp.Or(cmp.Compare(boolInt(b.Ready == 0),
			boolInt(a.Ready == 0)),
			cmp.Compare(len(b.Address), len(a.Address)),
			strings.Compare(a.Address, b.Address))
	})
	if len(out) > maxTraffic {
		out = out[:maxTraffic]
	}
	return out
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// addressText is "host/path", the bare host for the root path, or ""
// for a host that is not one.
func addressText(host, path string) string {
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
