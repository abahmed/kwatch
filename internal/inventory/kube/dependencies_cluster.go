package kube

import (
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrServiceCalls lists the in-cluster Services a pod is configured to
// call, as sorted "namespace/service:port" entries joined by commas (the
// ":port" is left out when the value names none). The port is what a
// NetworkPolicy check needs; the calls relation only keeps the Service.
const AttrServiceCalls = "calls.services"

// envHints are the words in an environment variable name that make a
// bare value such as "redis" an address rather than a mode or a flag.
var envHints = []string{"HOST", "ADDR", "SERVER", "ENDPOINT", "URL", "URI",
	"DSN", "BROKER", "SERVICE"}

// serviceCall is one in-cluster Service a pod's environment names.
type serviceCall struct {
	id   inventory.EntityID
	port string
}

func (c serviceCall) String() string {
	text := c.id.Namespace + "/" + c.id.Name
	if c.port != "" {
		text += ":" + c.port
	}
	return text
}

// podServiceCalls lists the in-cluster Services the pod's containers are
// configured to call, read from environment values: "redis:6379",
// "http://api.shop.svc:8080", or a bare "redis" in a variable named like
// REDIS_HOST. Values from Secrets and ConfigMaps are never read. The
// first maxPodDependencies by name are kept.
func podServiceCalls(pod *corev1.Pod) []serviceCall {
	seen := map[string]bool{}
	var out []serviceCall
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		for _, env := range c.Env {
			call, ok := serviceCallIn(env.Name, env.Value, pod.Namespace)
			if ok && !seen[call.String()] {
				seen[call.String()] = true
				out = append(out, call)
			}
		}
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	if len(out) > maxPodDependencies {
		out = out[:maxPodDependencies]
	}
	return out
}

// serviceCallIn reads the Service one environment value names.
func serviceCallIn(name, value, namespace string) (serviceCall, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, ",") {
		return serviceCall{}, false
	}
	host, port, bare := splitAddress(value)
	if host == "" || (bare && !hintedName(name)) {
		return serviceCall{}, false
	}
	ns, svc, ok := ClusterServiceName(host, namespace)
	if !ok {
		return serviceCall{}, false
	}
	return serviceCall{id: inventory.CoreID(KindService, ns, svc),
		port: port}, true
}

// splitAddress reads host and port from a URL or "host:port". bare
// reports a value that is only a host, with no scheme and no port.
func splitAddress(value string) (host, port string, bare bool) {
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return "", "", false
		}
		port = parsed.Port()
		if port == "" {
			port = defaultPorts[strings.ToLower(parsed.Scheme)]
		}
		return strings.ToLower(parsed.Hostname()), port, false
	}
	if match := hostPort.FindStringSubmatch(value); match != nil {
		return strings.ToLower(match[1]), match[2], false
	}
	if strings.ContainsAny(value, ":/@ ") {
		return "", "", false
	}
	return strings.ToLower(value), "", true
}

func hintedName(name string) bool {
	name = strings.ToUpper(name)
	for _, hint := range envHints {
		if strings.Contains(name, hint) {
			return true
		}
	}
	return false
}

// ClusterServiceName reads a host as an in-cluster Service name: "svc",
// "svc.ns", "svc.ns.svc" or "svc.ns.svc.<cluster domain>". A bare name
// belongs to namespace. Whether the Service exists is for the caller to
// check. IP addresses and "localhost" are never Services.
func ClusterServiceName(host, namespace string) (ns, name string, ok bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return "", "", false
	}
	labels := strings.Split(host, ".")
	switch {
	case len(labels) == 1:
		return namespace, labels[0], namespace != ""
	case len(labels) == 2:
		return labels[1], labels[0], true
	case len(labels) >= 3 && labels[2] == "svc":
		return labels[1], labels[0], true
	}
	return "", "", false
}

// serviceCallIDs are the Services of the calls, for the relation.
func serviceCallIDs(calls []serviceCall) []inventory.EntityID {
	out := make([]inventory.EntityID, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.id)
	}
	return out
}

// serviceCallsText is the attribute value for the calls.
func serviceCallsText(calls []serviceCall) string {
	parts := make([]string, len(calls))
	for i, call := range calls {
		parts[i] = call.String()
	}
	return strings.Join(parts, ",")
}

// ServiceRef is an in-cluster Service a pod is configured to call.
type ServiceRef struct {
	Service inventory.EntityID
	// Port is the Service port the configuration names; 0 when none.
	Port int
}

// ServiceCalls reads the Services a pod entity is configured to call
// from its AttrServiceCalls attribute.
func ServiceCalls(pod inventory.Entity) []ServiceRef {
	var out []ServiceRef
	for _, entry := range strings.Split(entityText(pod, AttrServiceCalls),
		",") {
		name, port, _ := strings.Cut(entry, ":")
		ns, svc, ok := strings.Cut(name, "/")
		if !ok {
			continue
		}
		number, _ := strconv.Atoi(port)
		out = append(out, ServiceRef{Service: inventory.CoreID(
			KindService, ns, svc), Port: number})
	}
	return out
}
