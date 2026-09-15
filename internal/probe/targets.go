package probe

import (
	"net"
	"net/url"
	"strings"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func (m *Monitor) linkTarget(owner, raw string) {
	if m.graph == nil {
		return
	}
	host := raw
	if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	} else if parsedHost, _, err := net.SplitHostPort(raw); err == nil {
		host = parsedHost
	}
	if svc, namespace, ok := serviceDNS(host); ok {
		m.graph.ReplaceOutgoingEdges(
			"activeprobe", "", owner,
			[]kwcontext.EdgeTarget{{
				Kind: "service", Namespace: namespace,
				Name: svc, Type: "probes",
			}},
		)
		return
	}
	m.graph.ReplaceOutgoingEdges(
		"activeprobe", "", owner,
		[]kwcontext.EdgeTarget{{
			Kind: "networktarget", Name: owner, Type: "probes",
		}},
	)
}

func serviceDNS(host string) (string, string, bool) {
	parts := strings.Split(strings.TrimSuffix(host, "."), ".")
	if len(parts) < 3 || parts[2] != "svc" ||
		parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
