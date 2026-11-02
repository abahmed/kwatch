package kube

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrPublicAddresses lists the addresses an Ingress or an HTTPRoute
// publishes, as "scheme<TAB>host<TAB>path<TAB>service<TAB>port" lines:
// one per host and path, with the Service and port it leads to in the
// object's own namespace. The scheme is https when TLS covers the host:
// an Ingress says so in spec.tls. An HTTPRoute's TLS ends at its
// Gateway, so its scheme is left empty (unknown) and so is the host of
// a route without hostnames: PublicAddresses fills both from the
// Gateway's listeners when the model holds them (gateway_address.go).
// Ingress rules without a host are left out: they have no address to
// name. Exact path matches other than "/" are left out too: they are
// no prefix a link can open pages below.
const AttrPublicAddresses = "public.addresses"

// KindHTTPRoute is the Gateway API HTTPRoute, watched when installed.
const KindHTTPRoute inventory.Kind = "httproute"

// PublicAddress is one entry of AttrPublicAddresses.
type PublicAddress struct {
	Scheme, Host, Path, Service, Port string
	// Source is the Ingress or HTTPRoute that publishes it.
	Source inventory.EntityID
}

// PublicAddresses returns what the Ingresses and HTTPRoutes of
// namespace publish, as the model holds it now.
// A route's address whose scheme stays unknown has an empty Scheme; one
// whose host stays unknown is left out.
func PublicAddresses(r inventory.Reader, namespace string) []PublicAddress {
	var out []PublicAddress
	values := r.AttributeIn(KindIngress, namespace, AttrPublicAddresses)
	for id, value := range values {
		out = append(out, parseAddresses(id, value.AsText())...)
	}
	values = r.AttributeIn(KindHTTPRoute, namespace, AttrPublicAddresses)
	for id, value := range values {
		listeners := routeListeners(r, id)
		for _, a := range parseAddresses(id, value.AsText()) {
			out = append(out, viaListeners(a, listeners)...)
		}
	}
	return out
}

func parseAddresses(id inventory.EntityID, text string) []PublicAddress {
	var out []PublicAddress
	for _, line := range strings.Split(text, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		out = append(out, PublicAddress{Scheme: f[0], Host: f[1],
			Path: f[2], Service: f[3], Port: f[4], Source: id})
	}
	return out
}

// setPublicAddresses records the addresses of an Ingress or HTTPRoute.
func setPublicAddresses(attrs map[string]inventory.Value, lines []string) {
	if len(lines) == 0 {
		return
	}
	sort.Strings(lines)
	lines = slices.Compact(lines)
	if len(lines) > maxPortEntries {
		lines = lines[:maxPortEntries]
	}
	attrs[AttrPublicAddresses] = inventory.Text(strings.Join(lines, "\n"))
}

// addressLine is one AttrPublicAddresses line; tabs and newlines in
// the fields become spaces.
func addressLine(scheme, host, path, service, port string) string {
	clean := strings.NewReplacer("\t", " ", "\n", " ")
	if path == "" {
		path = "/"
	}
	return strings.Join([]string{scheme, clean.Replace(host),
		clean.Replace(path), clean.Replace(service),
		clean.Replace(port)}, "\t")
}

// ingressAddresses lists the hosts and paths of an Ingress.
func ingressAddresses(ing *networkingv1.Ingress) []string {
	var lines []string
	for _, rule := range ing.Spec.Rules {
		if rule.Host == "" || rule.HTTP == nil {
			continue
		}
		scheme := "http"
		if ingressTLSCovers(ing, rule.Host) {
			scheme = "https"
		}
		for _, path := range rule.HTTP.Paths {
			svc := path.Backend.Service
			if svc == nil || svc.Port.Name == actionBackendPort ||
				exactNotRoot(path) {
				continue
			}
			port := svc.Port.Name
			if port == "" {
				port = strconv.Itoa(int(svc.Port.Number))
			}
			lines = append(lines, addressLine(scheme, rule.Host,
				path.Path, svc.Name, port))
		}
	}
	return lines
}

// ingressTLSCovers reports whether a TLS entry of ing serves host. An
// entry without hosts covers every host; "*.example.com" covers one
// label below example.com.
func ingressTLSCovers(ing *networkingv1.Ingress, host string) bool {
	for _, tls := range ing.Spec.TLS {
		if len(tls.Hosts) == 0 {
			return true
		}
		for _, h := range tls.Hosts {
			if hostMatches(h, host) {
				return true
			}
		}
	}
	return false
}

// exactNotRoot reports an Exact path other than "/": it matches that
// one path, so no page below it can be reached.
func exactNotRoot(path networkingv1.HTTPIngressPath) bool {
	return path.PathType != nil &&
		*path.PathType == networkingv1.PathTypeExact &&
		path.Path != "/" && path.Path != ""
}

func hostMatches(pattern, host string) bool {
	pattern, host = strings.ToLower(pattern), strings.ToLower(host)
	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		label, rest, found := strings.Cut(host, ".")
		return found && label != "" && rest == suffix
	}
	return pattern == host
}

// routeAddresses lists the hostnames and path matches of an HTTPRoute
// whose backends are Services of its own namespace. A route without
// hostnames takes its Gateway listener's, so its host is left empty.
func routeAddresses(u *unstructured.Unstructured) []string {
	hosts, _, _ := unstructured.NestedStringSlice(u.Object,
		"spec", "hostnames")
	if len(hosts) == 0 {
		hosts = []string{""}
	}
	rules, _, _ := unstructured.NestedSlice(u.Object, "spec", "rules")
	var lines []string
	for _, rule := range rules {
		ruleMap, _ := rule.(map[string]any)
		paths := routePaths(ruleMap)
		for _, backend := range ruleServices(u, ruleMap) {
			for _, host := range hosts {
				if strings.Contains(host, "*") {
					continue
				}
				for _, path := range paths {
					lines = append(lines, addressLine("", host, path,
						backend[0], backend[1]))
				}
			}
		}
	}
	return lines
}

// routePaths are the path prefixes a route rule matches; "/" for a
// match without a path, and for a rule without matches. Exact paths
// other than "/" and regular expressions are no prefix and are left
// out.
func routePaths(rule map[string]any) []string {
	matches, _, _ := unstructured.NestedSlice(rule, "matches")
	if len(matches) == 0 {
		return []string{"/"}
	}
	var out []string
	for _, m := range matches {
		match, _ := m.(map[string]any)
		if path, ok := routePrefix(match); ok {
			out = append(out, path)
		}
	}
	return out
}

// routePrefix is the path prefix one route match covers, if any. A
// match type defaults to PathPrefix and a value to "/".
func routePrefix(match map[string]any) (string, bool) {
	value, found, _ := unstructured.NestedString(match, "path", "value")
	if !found || value == "" {
		value = "/"
	}
	kind, _, _ := unstructured.NestedString(match, "path", "type")
	switch kind {
	case "", "PathPrefix":
		return value, true
	case "Exact":
		return value, value == "/"
	}
	return "", false
}

// ruleServices are the name and port of each Service a route rule
// sends to in the route's own namespace.
func ruleServices(
	u *unstructured.Unstructured, rule map[string]any,
) [][2]string {
	refs, _, _ := unstructured.NestedSlice(rule, "backendRefs")
	var out [][2]string
	for _, ref := range refs {
		m, _ := ref.(map[string]any)
		name, _ := m["name"].(string)
		kind, _ := m["kind"].(string)
		ns, _ := m["namespace"].(string)
		port, found, _ := unstructured.NestedFieldNoCopy(m, "port")
		if name == "" || (kind != "" && kind != "Service") ||
			(ns != "" && ns != u.GetNamespace()) || !found {
			continue
		}
		out = append(out, [2]string{name, portText(port)})
	}
	return out
}

// portText writes a port number however the decoder typed it.
func portText(port any) string {
	switch p := port.(type) {
	case int64:
		return strconv.FormatInt(p, 10)
	case float64:
		return strconv.FormatFloat(p, 'f', -1, 64)
	case int:
		return strconv.Itoa(p)
	}
	return ""
}
