package app

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dashboardPortName is the name the chart and the installer give the
// dashboard port of the dashboard Service.
const dashboardPortName = "dashboard"

// dashboardTarget is kwatch's own dashboard Service: the chart and the
// installer name it "<deployment>-dashboard", in kwatch's namespace,
// with a port named "dashboard" that leads to the dashboard port.
type dashboardTarget struct {
	namespace, service string
	port               int
}

func newDashboardTarget(port int) dashboardTarget {
	return dashboardTarget{
		namespace: envOrKwatch("POD_NAMESPACE"),
		service:   envOrKwatch(dashboardDeploymentEnv) + "-dashboard",
		port:      port,
	}
}

// schemeSource is what the running dashboard knows about how people
// reach it (dashboard.Server).
type schemeSource interface {
	// SchemeFor is the scheme the latest request for host came in with.
	SchemeFor(host string) (string, bool)
	// RequiresHTTPS is true when sign-in refuses plain http.
	RequiresHTTPS() bool
}

// address is the dashboard's public address, such as
// https://kwatch.example.com or https://ops.example.com/kwatch, read
// from the Ingresses and HTTPRoutes in kwatch's namespace that send to
// its dashboard Service. It is read from the model each time, so it
// follows those objects as they change. Among several, one with https
// wins, then the first by host, path and object. Exact paths other than
// "/" are not offered (see kube.PublicAddresses). "" when none does.
// seen may be nil; see linkScheme for how it picks the scheme.
func (d dashboardTarget) address(
	r inventory.Reader, seen schemeSource,
) string {
	address, _ := d.lookup(r, seen)
	return address
}

// lookup is address with, when it is "", the reason: the closest miss
// among the routes of kwatch's namespace.
func (d dashboardTarget) lookup(
	r inventory.Reader, seen schemeSource,
) (string, string) {
	ports := d.servicePorts(r)
	var found []kube.PublicAddress
	reason := health.LinksNoRoute
	for _, a := range kube.PublicAddresses(r, d.namespace) {
		switch {
		case a.Service != d.service:
		case !ports[a.Port]:
			if reason == health.LinksNoRoute {
				reason = health.LinksPortMismatch
			}
		case !usableHost(a.Host) || !usablePath(a.Path):
			reason = health.LinksUnusableAddress
		default:
			a.Scheme = linkScheme(a, seen)
			found = append(found, a)
		}
	}
	if len(found) == 0 {
		return "", reason
	}
	sort.Slice(found, func(i, j int) bool {
		return addressLess(found[i], found[j])
	})
	best := found[0]
	return best.Scheme + "://" + best.Host +
		strings.TrimRight(best.Path, "/"), ""
}

// linkScheme is the scheme of a link to address a, first match wins:
//  1. https when the object says so (TLS on the Ingress for the host,
//     or an HTTPS listener on the HTTPRoute's Gateway);
//  2. https when requests for the host were last seen over https (TLS
//     ended before the Ingress, by a load balancer that a trusted proxy
//     reports in X-Forwarded-Proto);
//  3. https when sign-in refuses plain http (shared mode), since an
//     http link could not be signed in to;
//  4. for an HTTPRoute whose Gateway is not in the model (scheme
//     unknown), the scheme requests were seen with, else https, since
//     Gateways usually end TLS;
//  5. otherwise http, as the object says and requests (if any) show.
func linkScheme(a kube.PublicAddress, seen schemeSource) string {
	if a.Scheme == "https" {
		return a.Scheme
	}
	if seen == nil {
		return knownScheme(a.Scheme)
	}
	scheme, ok := seen.SchemeFor(a.Host)
	if ok && scheme == "https" {
		return "https"
	}
	if seen.RequiresHTTPS() {
		return "https"
	}
	if a.Scheme == "" && ok && scheme != "" {
		return scheme
	}
	return knownScheme(a.Scheme)
}

// knownScheme is scheme, or https when it is unknown.
func knownScheme(scheme string) string {
	if scheme == "" {
		return "https"
	}
	return scheme
}

func addressLess(a, b kube.PublicAddress) bool {
	if (a.Scheme == "https") != (b.Scheme == "https") {
		return a.Scheme == "https"
	}
	if a.Host != b.Host {
		return a.Host < b.Host
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Source.String() < b.Source.String()
}

// servicePorts are the backend ports, by number or name, that reach the
// dashboard: the port named "dashboard" and the dashboard port number,
// plus every port of the Service in the model that targets either.
func (d dashboardTarget) servicePorts(r inventory.Reader) map[string]bool {
	number := strconv.Itoa(d.port)
	ports := map[string]bool{dashboardPortName: true, number: true}
	id := inventory.CoreID(kube.KindService, d.namespace, d.service)
	attr, ok := r.Attribute(id, kube.AttrServicePortSpecs)
	if !ok {
		return ports
	}
	for _, spec := range strings.Split(attr.Value.AsText(), ",") {
		f := strings.SplitN(spec, ":", 3)
		if len(f) != 3 || (f[2] != number && f[2] != dashboardPortName) {
			continue
		}
		ports[f[0]] = true
		if f[1] != "" {
			ports[f[1]] = true
		}
	}
	return ports
}

// usableHost is a single DNS name: no wildcard, port or path.
func usableHost(host string) bool {
	return host != "" && !strings.ContainsAny(host, "*/:@ ")
}

// usablePath is a plain path prefix. A regular expression (used with a
// rewrite) names no address a link can open.
func usablePath(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	for _, c := range path {
		if !isPathChar(c) {
			return false
		}
	}
	return true
}

func isPathChar(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || strings.ContainsRune("/-._~", c)
}
