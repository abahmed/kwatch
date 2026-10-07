package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// portMismatch reports a Service whose targetPort leads nowhere on the
// pods it selects. A target NAME no pod declares is definitive: the
// endpoints controller leaves those pods out of the port. A target
// NUMBER is only a mismatch when the pods declare ports and none
// matches AND the Service is being refused right now, because a
// container may listen on a port it never declared.
func portMismatch(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	ports := parseServicePorts(text(e, kube.AttrServicePortSpecs))
	if ctx.Model == nil || len(ports) == 0 || !ctx.Synced(kube.KindPod) {
		return nil
	}
	var declared []podPort
	pods := kube.SelectedPods(ctx.Model, e)
	for _, id := range pods {
		if pod, ok := ctx.Model.Entity(id); ok {
			declared = append(declared,
				parsePodPorts(text(pod, kube.AttrPodPorts))...)
		}
	}
	if len(pods) == 0 {
		return nil
	}
	for _, port := range ports {
		summary, ok := unmatchedTarget(e, port, declared)
		if !ok {
			continue
		}
		since := ctx.Onset("port-mismatch",
			valueSince(e, kube.AttrServicePortSpecs))
		if !sustained(ctx, "port-mismatch", since, DefaultNoEndpoints) {
			return nil
		}
		return []detection.Finding{{
			Reason: reasons.ServicePortMismatch, Severity: detection.Critical,
			Since: since, Summary: summary,
			Evidence: []detection.Evidence{
				{Label: "service port", Value: port.number + " -> " +
					port.target},
				{Label: "pod ports", Value: describePodPorts(declared)},
			},
		}}
	}
	return nil
}

// unmatchedTarget words the mismatch of one Service port, when there is
// one that is certain enough to report.
func unmatchedTarget(
	e inventory.Entity, port servicePort, declared []podPort,
) (string, bool) {
	for _, have := range declared {
		if have.number == port.target ||
			(have.name != "" && have.name == port.target) {
			return "", false
		}
	}
	if !isPortNumber(port.target) {
		return "sends traffic to the port named " + port.target +
			", but none of its pods declares a port with that name" +
			ofPods(declared) + ". " + e.ID.Name +
			" gets no endpoints for that port", true
	}
	if len(declared) == 0 || !refusingConnections(e) {
		return "", false
	}
	return "sends traffic to port " + port.target +
		", but its pods listen on " + describePodPorts(declared) +
		". Connections to " + e.ID.Name + " are refused", true
}

// ofPods adds what the pods do declare, when they declare anything.
func ofPods(declared []podPort) string {
	if len(declared) == 0 {
		return " (they declare no ports)"
	}
	return " (they listen on " + describePodPorts(declared) + ")"
}

// refusingConnections reports the Service's own probe being refused
// right now.
func refusingConnections(e inventory.Entity) bool {
	healthy, known := e.Attribute(kube.AttrHealthy)
	if !known {
		return false
	}
	if ok, _ := healthy.Value.AsBool(); ok {
		return false
	}
	return strings.TrimSpace(text(e, kube.AttrProbeFailureKind)) ==
		kube.FailureRefused
}
