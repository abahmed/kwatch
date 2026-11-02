package kube

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrGatewayListeners lists the HTTP and HTTPS listeners of a Gateway
// API Gateway as "protocol<TAB>hostname" lines; the hostname is empty
// when the listener serves every host.
const AttrGatewayListeners = "gateway.listeners"

// KindGateway is the Gateway API Gateway, watched when installed.
const KindGateway inventory.Kind = "gateway"

// listener is one entry of AttrGatewayListeners.
type listener struct{ protocol, hostname string }

// setAddressAttributes records what a Gateway API object tells about
// public addresses: an HTTPRoute's addresses, a Gateway's listeners.
func (s UnstructuredSchema) setAddressAttributes(
	u *unstructured.Unstructured, attrs map[string]inventory.Value,
) {
	if s.group != gatewayGroup {
		return
	}
	switch s.kind {
	case KindHTTPRoute:
		setPublicAddresses(attrs, routeAddresses(u))
	case KindGateway:
		if lines := gatewayListeners(u); len(lines) > 0 {
			attrs[AttrGatewayListeners] = inventory.Text(
				strings.Join(lines, "\n"))
		}
	}
}

// gatewayListeners lists the HTTP and HTTPS listeners of a Gateway,
// sorted and bounded.
func gatewayListeners(u *unstructured.Unstructured) []string {
	items, _, _ := unstructured.NestedSlice(u.Object, "spec", "listeners")
	clean := strings.NewReplacer("\t", " ", "\n", " ")
	var lines []string
	for _, item := range items {
		m, _ := item.(map[string]any)
		protocol, _ := m["protocol"].(string)
		hostname, _ := m["hostname"].(string)
		if protocol != "HTTP" && protocol != "HTTPS" {
			continue
		}
		lines = append(lines, protocol+"\t"+clean.Replace(hostname))
	}
	slices.Sort(lines)
	lines = slices.Compact(lines)
	return lines[:min(len(lines), maxPortEntries)]
}

// routeListeners are the listeners of the Gateways a route attaches to,
// as far as the model holds them.
func routeListeners(r inventory.Reader, route inventory.EntityID) []listener {
	var out []listener
	parents := r.Related(route, inventory.References, inventory.Outgoing)
	for _, gw := range parents {
		if gw.Kind != KindGateway || gw.Group != GroupFor(gatewayGroup) {
			continue
		}
		attr, ok := r.Attribute(gw, AttrGatewayListeners)
		if !ok {
			continue
		}
		for _, line := range strings.Split(attr.Value.AsText(), "\n") {
			protocol, hostname, found := strings.Cut(line, "\t")
			if found {
				out = append(out, listener{protocol, hostname})
			}
		}
	}
	return out
}

// viaListeners completes a route's address from its Gateway listeners:
// the scheme is the listener's protocol, and a route without hostnames
// takes the listener's hostname. Without a known listener the scheme
// stays unknown, and an address without a host is left out.
func viaListeners(a PublicAddress, listeners []listener) []PublicAddress {
	var out []PublicAddress
	for _, l := range listeners {
		b, ok := onListener(a, l)
		if ok && !slices.Contains(out, b) {
			out = append(out, b)
		}
	}
	if len(out) == 0 && a.Host != "" {
		out = append(out, a)
	}
	return out
}

// onListener is address a as served by listener l, if l serves it.
func onListener(a PublicAddress, l listener) (PublicAddress, bool) {
	switch {
	case a.Host == "" && (l.hostname == "" ||
		strings.Contains(l.hostname, "*")):
		return a, false
	case a.Host == "":
		a.Host = l.hostname
	case l.hostname != "" && !hostMatches(l.hostname, a.Host):
		return a, false
	}
	a.Scheme = "http"
	if l.protocol == "HTTPS" {
		a.Scheme = "https"
	}
	return a, true
}
