package kube

import (
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// defaultPorts are the ports a URL scheme implies when it names none.
var defaultPorts = map[string]string{
	"postgres": "5432", "postgresql": "5432", "mysql": "3306",
	"redis": "6379", "rediss": "6379", "amqp": "5672", "amqps": "5671",
	"mongodb": "27017", "kafka": "9092", "nats": "4222",
	"memcached": "11211", "ldap": "389", "ldaps": "636", "smtp": "25",
	"smtps": "465", "http": "80", "https": "443", "grpc": "443",
}

// hostPort matches a bare "host:port" value.
var hostPort = regexp.MustCompile(`^([A-Za-z0-9.-]+):(\d{2,5})$`)

// clusterSuffixes end the names of in-cluster Services. They are
// Kubernetes objects kwatch already watches, not external dependencies.
var clusterSuffixes = []string{".svc", ".svc.cluster.local", ".cluster.local"}

// podDependencies lists the external endpoints a pod's containers are
// configured to call, read from environment values that look like a
// URL or a host:port: "postgres://db.example.com:5432/orders" or
// "cache.example.com:6379". Only the host and port are kept; user
// names, passwords, paths and queries never leave the value. Hosts
// without a dot, in-cluster Service names, localhost and IP literals
// are left out: they name nothing outside the cluster.
func podDependencies(pod *corev1.Pod) []inventory.EntityID {
	seen := map[string]bool{}
	var out []inventory.EntityID
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		for _, env := range c.Env {
			endpoint, ok := dependencyIn(env.Value)
			if !ok || seen[endpoint] {
				continue
			}
			seen[endpoint] = true
			out = append(out, inventory.CoreID(KindExternalEndpoint, "",
				endpoint))
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// dependencyIn reads the "host:port" an environment value names, if it
// names one outside the cluster.
func dependencyIn(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	host, port := "", ""
	switch {
	case strings.Contains(value, "://"):
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" {
			return "", false
		}
		host, port = parsed.Hostname(), parsed.Port()
		if port == "" {
			port = defaultPorts[strings.ToLower(parsed.Scheme)]
		}
	default:
		match := hostPort.FindStringSubmatch(value)
		if match == nil {
			return "", false
		}
		host, port = match[1], match[2]
	}
	if port == "" || !externalHost(host) {
		return "", false
	}
	return net.JoinHostPort(strings.ToLower(host), port), true
}

// externalHost reports a host name that points outside the cluster: a
// dotted name that is not an in-cluster Service, localhost or an IP.
func externalHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || host == "localhost" || !strings.Contains(host, ".") ||
		net.ParseIP(host) != nil {
		return false
	}
	for _, suffix := range clusterSuffixes {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	return true
}
