package detectors

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
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
	out := append(loadBalancerPending(ctx, e),
		loadBalancerEvents(ctx, e)...)
	if text(e, kube.AttrSelector) == "" ||
		text(e, kube.AttrServiceType) == "ExternalName" {
		return out
	}
	return append(out, backendFindings(ctx, e)...)
}

// DefaultLoadBalancerPending is how long a LoadBalancer may wait for an
// address; cloud load balancers normally provision within minutes.
const DefaultLoadBalancerPending = 5 * time.Minute

func loadBalancerPending(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if text(e, kube.AttrServiceType) != "LoadBalancer" ||
		flag(e, kube.AttrLoadBalancer) {
		return nil
	}
	since := valueSince(e, kube.AttrLoadBalancer)
	if !sustained(ctx, "lb-pending", since, DefaultLoadBalancerPending) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.LoadBalancerPending, Severity: detection.Warning,
		Since:   since,
		Summary: "LoadBalancer Service has no external address yet",
	}}
}

func backendFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if text(e, kube.AttrSelector) == "" ||
		text(e, kube.AttrServiceType) == "ExternalName" {
		return nil
	}
	slices := ctx.Model.Related(e.ID, inventory.Backs, inventory.Incoming)
	if len(slices) == 0 {
		return nil
	}
	endpoints, ready, since := sumEndpoints(ctx, slices)
	var out []detection.Finding
	if s, ok := portMismatch(ctx, e, slices); ok {
		out = append(out, s)
	}
	// No endpoints at all means nothing is selected (scaled to zero on
	// purpose); unready endpoints mean selected pods are failing.
	switch {
	case endpoints == 0:
		if s, ok := selectsNothing(ctx, e); ok {
			out = append(out, s)
		}
		return out
	case ready == 0:
		since = ctx.Onset("no-ready-backends", since)
		if sustained(ctx, "no-ready-backends", since, DefaultNoEndpoints) {
			out = append(out, detection.Finding{
				Reason:   reasons.ServiceNoEndpoints,
				Severity: detection.Critical, Since: since, Symptom: true,
				Summary: "Service has no ready backends; traffic to it " +
					"fails",
			})
		}
	case ready < endpoints:
		// Losing one more backend must not restart the wait.
		since = ctx.Onset("degraded-backends", since)
		if sustained(ctx, "degraded-backends", since, DefaultDegradedBackends) {
			out = append(out, detection.Finding{
				Reason:   reasons.ServiceBackendsDegraded,
				Severity: detection.Warning, Since: since, Symptom: true,
				Summary: strconv.Itoa(int(ready)) + " of " +
					strconv.Itoa(int(endpoints)) +
					" Service backends are ready",
			})
		}
	}
	return out
}

// sumEndpoints totals endpoints and ready endpoints over the slices and
// returns the latest time the ready count changed. That is when the
// current count began, not when the bad state began; callers use it only
// the first time the bad state is seen.
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
		endpoints += total
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

// portMismatch reports numeric target ports that no EndpointSlice
// publishes. Named target ports resolve per pod and are not checked.
func portMismatch(
	ctx detection.Context, e inventory.Entity, slices []inventory.EntityID,
) (detection.Finding, bool) {
	published := map[string]bool{}
	for _, id := range slices {
		slice, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if up, _ := number(slice, kube.AttrEndpoints); up == 0 {
			continue
		}
		for _, port := range strings.Split(
			text(slice, kube.AttrEndpointPorts), ",") {
			published[port] = true
		}
	}
	if len(published) == 0 {
		return detection.Finding{}, false
	}
	var missing []string
	for _, port := range strings.Split(text(e, kube.AttrTargetPorts), ",") {
		if _, err := strconv.Atoi(port); err == nil && !published[port] {
			missing = append(missing, port)
		}
	}
	if len(missing) == 0 {
		return detection.Finding{}, false
	}
	since := valueSince(e, kube.AttrTargetPorts)
	if !sustained(ctx, "missing-target-ports", since, DefaultNoEndpoints) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.ServicePortMismatch, Severity: detection.Warning,
		Since: since,
		Summary: "Service targets port " + strings.Join(missing, ", ") +
			", which its pods do not expose",
	}, true
}

// Ingress detects Ingresses whose backend Service, TLS Secret or
// IngressClass does not exist.
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
	out := ingressBackends(ctx, e)
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
	if len(missing) == 0 {
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
