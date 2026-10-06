package detectors

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// scaledToZeroGrace is how long a workload may stay at zero replicas
// while routed to before it is reported: a deliberate pause or a
// rollout that starts from zero is over sooner. With the settle it keeps
// the first message inside the five minutes the alert-quality goal allows.
const scaledToZeroGrace = 3 * time.Minute

// scaleChangeWindow is how far back the history is searched for who
// scaled the workload.
const scaleChangeWindow = 7 * 24 * time.Hour

// routeKindNames are the names people use for the kinds of routes.
var routeKindNames = map[inventory.Kind]string{
	"ingress": "Ingress", "httproute": "HTTPRoute",
	"grpcroute": "GRPCRoute", "tlsroute": "TLSRoute",
}

// exposure is one way traffic still reaches a Service: a route, or the
// Service's own type.
type exposure struct {
	service inventory.EntityID
	// route is the Ingress or Gateway route sending traffic to the
	// Service; zero when the Service is exposed by its type.
	route       inventory.EntityID
	serviceType string
}

// scaledToZeroRouted reports a Deployment or StatefulSet scaled to zero
// replicas whose Service still receives traffic, from an Ingress or a
// Gateway route or by being a LoadBalancer or NodePort. Every request
// ends in an error. An autoscaler that may scale it to zero (minimum 0)
// makes zero a normal state, which is left alone.
func scaledToZeroRouted(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	replicas, ok := number(e, kube.AttrReplicas)
	if !ok || replicas != 0 || ctx.Model == nil ||
		(e.ID.Kind != kube.KindDeployment &&
			e.ID.Kind != kube.KindStatefulSet) ||
		scalesToZeroByDesign(ctx, e.ID) {
		return detection.Finding{}, false
	}
	exposed := exposures(ctx, e)
	if len(exposed) == 0 {
		return detection.Finding{}, false
	}
	scaled := scaledBy(ctx, e.ID)
	since := valueSince(e, kube.AttrReplicas)
	if !scaled.At.IsZero() {
		since = scaled.At
	}
	if !sustained(ctx, "scaled-to-zero", since, scaledToZeroGrace) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.ScaledToZeroRouted, Severity: detection.Warning,
		Since:    since,
		Summary:  "Is scaled to 0 but " + exposureText(exposed[0]),
		Evidence: scaleEvidence(exposed, scaled),
	}, true
}

// scalesToZeroByDesign is a workload an autoscaler may scale to zero or
// has parked there (HPA condition ScalingActive=False, ScalingDisabled).
func scalesToZeroByDesign(
	ctx detection.Context, workload inventory.EntityID,
) bool {
	for _, id := range ctx.Model.Related(workload, inventory.Scales,
		inventory.Incoming) {
		hpa, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if floor, known := number(hpa, kube.AttrMinReplicas); known &&
			floor == 0 {
			return true
		}
		// An autoscaler that parked the target at zero says so with the
		// standard ScalingActive=False, reason ScalingDisabled.
		status, reason, _ := condition(hpa, "ScalingActive")
		if status == "False" && reason == reasons.ScalingDisabled {
			return true
		}
	}
	return false
}

// exposures are the Services of the workload that traffic still
// reaches, routes first, each in name order. A Service with ready
// endpoints is skipped: something else is serving it.
func exposures(
	ctx detection.Context, e inventory.Entity,
) []exposure {
	var out []exposure
	for _, service := range kube.ServicesSelecting(ctx.Model, e) {
		if readyBackends(ctx, service) > 0 {
			// Another workload (a blue/green twin) still serves it.
			continue
		}
		routes := ctx.Model.Related(service, inventory.RoutesTo,
			inventory.Incoming)
		for _, route := range routes {
			out = append(out, exposure{service: service, route: route})
		}
		if len(routes) > 0 {
			continue
		}
		if entity, ok := ctx.Model.Entity(service); ok {
			kind := text(entity, kube.AttrServiceType)
			if kind == "LoadBalancer" || kind == "NodePort" {
				out = append(out, exposure{service: service,
					serviceType: kind})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return exposureKey(out[i]) < exposureKey(out[j])
	})
	return out
}

func exposureKey(x exposure) string {
	return x.route.String() + "|" + x.service.String()
}

// exposureText says what still sends traffic: "Ingress shop/web still
// routes traffic to it via Service api".
func exposureText(x exposure) string {
	if x.route.IsZero() {
		return "Service " + x.service.Name + " (" + x.serviceType +
			") still exposes it"
	}
	return routeKindName(x.route.Kind) + " " + x.route.Namespace + "/" +
		x.route.Name + " still routes traffic to it via Service " +
		x.service.Name
}

func routeKindName(kind inventory.Kind) string {
	if name, ok := routeKindNames[kind]; ok {
		return name
	}
	return string(kind)
}

// scaledBy is the latest change that set the replicas to zero: who made
// it (the field manager's name, as recorded) and when.
func scaledBy(
	ctx detection.Context, workload inventory.EntityID,
) inventory.Change {
	changes := ctx.Model.Changes(workload, ctx.Now.Add(-scaleChangeWindow))
	for i := len(changes) - 1; i >= 0; i-- {
		for _, field := range changes[i].Fields {
			if field.Path == "spec.replicas" && field.After == "0" {
				return changes[i]
			}
		}
	}
	return inventory.Change{}
}

// scaleEvidence lists what still routes traffic and, when the history
// knows, who scaled the workload and when.
func scaleEvidence(
	exposed []exposure, scaled inventory.Change,
) []detection.Evidence {
	var out []detection.Evidence
	for _, x := range exposed {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceStillRouted, Value: exposureText(x)})
	}
	if scaled.Actor != "" {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceScaledBy, Value: scaled.Actor})
	}
	if !scaled.At.IsZero() {
		out = append(out, detection.Evidence{Label: detection.
			EvidenceScaledAt, Value: scaled.At.UTC().Format(time.RFC3339)})
	}
	return out
}

// readyBackends counts the ready endpoints behind a Service, summed over
// its EndpointSlices. A Service with some is still answering requests.
func readyBackends(
	ctx detection.Context, service inventory.EntityID,
) float64 {
	var ready float64
	slices := ctx.Model.Related(service, inventory.Backs, inventory.Incoming)
	for _, id := range slices {
		if slice, ok := ctx.Model.Entity(id); ok {
			up, _ := number(slice, kube.AttrEndpointsReady)
			ready += up
		}
	}
	return ready
}
