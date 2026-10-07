package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ingressBackendPorts reports an Ingress rule whose backend port, a
// number or a name, is not a port of the Service it routes to. The
// Service exists (a missing one is reported by ingressBackends), so the
// routing controller has nothing to send the traffic to.
func ingressBackendPorts(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || !ctx.Synced(kube.KindService) {
		return nil
	}
	for _, line := range strings.Split(
		text(e, kube.AttrIngressBackendPorts), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		path, name, want := fields[0], fields[1], fields[2]
		svc, ok := ctx.Model.Entity(inventory.CoreID(
			kube.KindService, e.ID.Namespace, name))
		ports := parseServicePorts(text(svc, kube.AttrServicePortSpecs))
		if !ok || len(ports) == 0 || hasServicePort(ports, want) {
			continue
		}
		if !sustained(ctx, "backend-port", e.FirstSeen,
			DefaultBackendGrace) {
			return nil
		}
		return []detection.Finding{{
			Reason:   reasons.IngressBackendPortMissing,
			Severity: detection.Critical,
			Summary: "Ingress routes " + routeWords(path) + " to Service " +
				name + " port " + want + ", which " + name +
				" doesn't have (it has " + describePorts(ports) + ")",
			Evidence: []detection.Evidence{
				{Label: "backend", Value: name + ":" + want},
				{Label: "service ports", Value: describePorts(ports)},
			},
		}}
	}
	return nil
}

// routeWords names what the rule matches.
func routeWords(path string) string {
	if path == "" {
		return "its default backend"
	}
	return path
}
