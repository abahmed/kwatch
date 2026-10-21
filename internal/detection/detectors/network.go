package detectors

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultNoEndpoints is how long a Service may have no ready endpoints;
// rolling restarts briefly empty small Services.
const DefaultNoEndpoints = time.Minute

// Service detects Services that select pods but route to none of them.
// The finding is a symptom: its cause is whatever made the pods unready.
type Service struct{}

// Name implements detection.Detector.
func (Service) Name() string { return "service" }

// Kinds implements detection.Detector.
func (Service) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindService}
}

// Detect implements detection.Detector.
func (Service) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	out := append(loadBalancerWaiting(ctx, e),
		loadBalancerEvents(ctx, e)...)
	if text(e, kube.AttrSelector) == "" ||
		text(e, kube.AttrServiceType) == "ExternalName" {
		return out
	}
	// Endpoints that are missing or unready because the port leads
	// nowhere are that mismatch's consequence, not a second failure.
	if mismatch := portMismatch(ctx, e); len(mismatch) > 0 {
		return append(out, mismatch...)
	}
	return append(out, backendFindings(ctx, e)...)
}

func backendFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if text(e, kube.AttrSelector) == "" ||
		text(e, kube.AttrServiceType) == "ExternalName" {
		return nil
	}
	if !ctx.Synced(kube.KindEndpointSlice) {
		return nil
	}
	slices := ctx.Model.Related(e.ID, inventory.Backs, inventory.Incoming)
	if len(slices) == 0 {
		return nil
	}
	endpoints, ready, since := sumEndpoints(ctx, slices)
	// No endpoints at all means nothing is selected (scaled to zero on
	// purpose) or the selected pods are all Pending; unready endpoints
	// mean selected pods are failing.
	switch {
	case endpoints == 0:
		return noEndpoints(ctx, e, since)
	case ready == 0:
		return noReadyBackends(ctx, e, since)
	case ready < endpoints:
		// Losing one more backend must not restart the wait.
		since = ctx.Onset("degraded-backends", since)
		if sustained(ctx, "degraded-backends", since, DefaultDegradedBackends) {
			return []detection.Finding{{
				Reason:   reasons.ServiceBackendsDegraded,
				Severity: detection.Warning, Since: since, Symptom: true,
				Summary: strconv.Itoa(int(ready)) + " of " +
					strconv.Itoa(int(endpoints)) +
					" Service backends are ready",
			}}
		}
	}
	return nil
}

// noEndpoints judges a Service whose EndpointSlices list nothing. A
// selector edit, a selector that no pod or workload can ever match
// behind an exposure, or selected pods that are all Pending are
// failures; otherwise the Service is parked and only the digest hears
// of it after DefaultUnusedAfter.
func noEndpoints(
	ctx detection.Context, e inventory.Entity, since time.Time,
) []detection.Finding {
	if s, ok := selectsNothing(ctx, e); ok {
		return []detection.Finding{s}
	}
	if s, ok := selectorMatchesNothing(ctx, e); ok {
		return []detection.Finding{s}
	}
	if allSelectedPending(ctx, e) {
		return noReadyBackends(ctx, e, since)
	}
	if s, ok := unusedService(ctx, since); ok {
		return []detection.Finding{s}
	}
	return nil
}

// noReadyBackends is the Service-level symptom of pods that exist but
// serve nothing: it is raised once the state has held for
// DefaultNoEndpoints, unless the pods are still starting.
func noReadyBackends(
	ctx detection.Context, e inventory.Entity, since time.Time,
) []detection.Finding {
	since = ctx.Onset("no-ready-backends", since)
	if backendsAreStarting(ctx, e.ID) ||
		!sustained(ctx, "no-ready-backends", since, DefaultNoEndpoints) {
		return nil
	}
	return []detection.Finding{{
		Reason:   reasons.ServiceNoEndpoints,
		Severity: detection.Critical, Since: since, Symptom: true,
		Summary: "Service has no ready backends; traffic to it fails",
	}}
}

// sumEndpoints totals the endpoints that are not terminating and the
// ready endpoints over the slices, and returns the latest time the
// ready count changed. That is when the current count began, not when
// the bad state began; callers use it only the first time the bad
// state is seen.
func sumEndpoints(
	ctx detection.Context, slices []inventory.EntityID,
) (endpoints, ready float64, since time.Time) {
	for _, id := range slices {
		slice, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		total, _ := number(slice, kube.AttrEndpoints)
		up, _ := number(slice, kube.AttrEndpointsReady)
		leaving, _ := number(slice, kube.AttrEndpointsTerminating)
		// A terminating endpoint is a pod shutting down on purpose: it
		// is neither a backend nor a failed one.
		endpoints += total - leaving
		ready += up
		if at := valueSince(slice, kube.AttrEndpointsReady); at.After(since) {
			since = at
		}
	}
	return endpoints, ready, since
}

// DefaultDegradedBackends is how long a Service may run with some unready
// backends; rolling updates pass through this state briefly.
const DefaultDegradedBackends = 5 * time.Minute

// Ingress detects Ingresses whose backend Service, TLS Secret or
// IngressClass does not exist, or that never get an address.
type Ingress struct{}

// Name implements detection.Detector.
func (Ingress) Name() string { return "ingress" }

// Kinds implements detection.Detector.
func (Ingress) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindIngress}
}

// Detect implements detection.Detector.
func (Ingress) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	out := loadBalancerWaiting(ctx, e)
	out = append(out, ingressBackends(ctx, e)...)
	out = append(out, ingressBackendPorts(ctx, e)...)
	out = append(out, ingressTLSSecrets(ctx, e)...)
	return append(out, ingressClass(ctx, e)...)
}

func ingressBackends(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	return missingBackends(ctx, e, "Ingress routes")
}

// routeBackends reports a Gateway API route that sends traffic to a
// Service that does not exist. The Gateway reports it as ResolvedRefs
// False (BackendNotFound), which the custom-resource detector only sees
// as a failed condition after its grace period; the missing Service is
// conclusive as soon as the Services are synced, as for an Ingress.
func routeBackends(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	return missingBackends(ctx, e, "Route sends traffic")
}

// missingBackends reports the Services an Ingress or route sends
// traffic to that do not exist. subject starts the summary.
func missingBackends(
	ctx detection.Context, e inventory.Entity, subject string,
) []detection.Finding {
	if ctx.Model == nil || !ctx.Synced(kube.KindService) {
		return nil
	}
	var missing []string
	for _, service := range ctx.Model.Related(
		e.ID, inventory.RoutesTo, inventory.Outgoing,
	) {
		// Routes may target ServiceImports or vendor backends, which
		// are not in the Service list.
		if service.Kind == kube.KindService && !ctx.Model.Exists(service) {
			missing = append(missing, service.Name)
		}
	}
	if len(missing) == 0 ||
		!sustained(ctx, "missing-backend", e.FirstSeen, DefaultBackendGrace) {
		return nil
	}
	return []detection.Finding{{
		Reason:   reasons.IngressBackendNotFound,
		Severity: detection.Critical,
		Summary: subject + " to Service " +
			strings.Join(missing, ", ") + ", which does not exist",
	}}
}

// EgressPolicy reports NetworkPolicies that block all outgoing traffic of
// the pods they select. This is often intended, so it is informational;
// the network-policy rule uses it as evidence when those pods fail.
type EgressPolicy struct{}

// Name implements detection.Detector.
func (EgressPolicy) Name() string { return "egress-policy" }

// Kinds implements detection.Detector.
func (EgressPolicy) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindNetworkPolicy}
}

// Detect implements detection.Detector.
func (EgressPolicy) Detect(
	_ detection.Context, e inventory.Entity,
) []detection.Finding {
	if !flag(e, kube.AttrDeniesEgress) {
		return nil
	}
	return []detection.Finding{{
		Reason:   reasons.RestrictiveNetworkPolicy,
		Severity: detection.Info,
		Since:    valueSince(e, kube.AttrDeniesEgress),
		Summary:  "Policy blocks all outgoing traffic of the pods it selects",
	}}
}

// DefaultUnusedAfter is how long a Service may select no pod, or a
// bound claim go unmounted, before it is reported as unused. A day
// covers nightly scale-downs and weekend quiet.
const DefaultUnusedAfter = 24 * time.Hour

// unusedService reports a Service whose selector has matched no pod for
// a day: clutter at best, a wrong selector at worst. It waits for the
// digest.
func unusedService(
	ctx detection.Context, since time.Time,
) (detection.Finding, bool) {
	since = ctx.Onset("unused", since)
	if !sustained(ctx, "unused", since, DefaultUnusedAfter) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.ServiceUnused, Severity: detection.Info, Since: since,
		Summary: "Service has selected no pod for " +
			format.Duration(ctx.Now.Sub(since)),
	}, true
}
