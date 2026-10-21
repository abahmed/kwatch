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

// scaledToZeroParked is how long a workload may have been at zero
// replicas before it counts as parked: its routes are left over from
// work nobody means to finish, so it is reported in the digest, not as
// news. A scale-down within this time still notifies.
const scaledToZeroParked = 24 * time.Hour

// scaleChangeWindow is how far back the history is searched for who
// scaled the workload: as far as the model keeps changes, since it
// prunes and reloads no more than that after a restart.
const scaleChangeWindow = inventory.ChangeRetention

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
	// A twin serving the Service shows only in its EndpointSlices.
	if !ctx.Synced(kube.KindEndpointSlice) {
		return detection.Finding{}, false
	}
	exposed := exposures(ctx, e)
	if len(exposed) == 0 {
		return detection.Finding{}, false
	}
	scaled := scaledBy(ctx, e.ID)
	if kube.ScaledDownInBatch(ctx.Model, e.ID, scaled.At) {
		// A planned scale-down put it at zero on purpose.
		return detection.Finding{}, false
	}
	since, known := scaledSince(e, scaled)
	if !sustained(ctx, "scaled-to-zero", since, scaledToZeroGrace) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason:   reasons.ScaledToZeroRouted,
		Severity: scaledSeverity(ctx, since, known),
		Since:    since,
		Summary:  scaledSummary(exposed[0], known),
		Evidence: scaleEvidence(exposed, scaled),
	}, true
}

// scaledSince is when the workload was scaled to zero, from the cluster
// when it says: the scale-down kwatch saw, else the latest spec write
// the API server recorded. Otherwise it is only known to be no later
// than kwatch's first sight of it, and known is false.
func scaledSince(
	e inventory.Entity, scaled inventory.Change,
) (since time.Time, known bool) {
	if !scaled.At.IsZero() {
		return scaled.At, true
	}
	if written := timestamp(e, kube.AttrSpecWritten); !written.IsZero() {
		return written, true
	}
	return valueSince(e, kube.AttrReplicas), false
}

// scaledSummary words the finding: a start kwatch cannot date says so,
// instead of passing its own first sight for the scale-down.
func scaledSummary(x exposure, known bool) string {
	if known {
		return "Is scaled to 0 but " + exposureText(x)
	}
	return "Is scaled to 0 (since before kwatch started) but " +
		exposureText(x)
}

// scaledSeverity is a warning while the scale-down is news and
// informational once the workload has been parked for scaledToZeroParked:
// its routes are then left over. A start kwatch cannot date is news. It
// asks to be evaluated again when the day is over.
func scaledSeverity(
	ctx detection.Context, since time.Time, known bool,
) detection.Severity {
	if !known {
		return detection.Warning
	}
	parked := ctx.Now.Sub(since)
	if parked >= scaledToZeroParked {
		return detection.Info
	}
	ctx.RecheckAfter(scaledToZeroParked - parked)
	return detection.Warning
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
		out = append(out, serviceExposures(ctx, service)...)
	}
	sort.Slice(out, func(i, j int) bool {
		return exposureKey(out[i]) < exposureKey(out[j])
	})
	return out
}

// serviceExposures are the ways traffic still reaches one Service: each
// route sending traffic to it, else its own LoadBalancer or NodePort
// type.
func serviceExposures(
	ctx detection.Context, service inventory.EntityID,
) []exposure {
	var out []exposure
	routes := ctx.Model.Related(service, inventory.RoutesTo,
		inventory.Incoming)
	for _, route := range routes {
		out = append(out, exposure{service: service, route: route})
	}
	if len(routes) > 0 {
		return out
	}
	if entity, ok := ctx.Model.Entity(service); ok {
		kind := text(entity, kube.AttrServiceType)
		if kind == "LoadBalancer" || kind == "NodePort" {
			out = append(out, exposure{service: service,
				serviceType: kind})
		}
	}
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
