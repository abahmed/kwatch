package detect

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultNoEndpoints is how long a Service may have no ready endpoints;
// rolling restarts briefly empty small Services.
const DefaultNoEndpoints = time.Minute

// Service detects Services that select pods but route to none of them.
// The signal is a symptom: its cause is whatever made the pods unready.
type Service struct{}

// Name implements signal.Detector.
func (Service) Name() string { return "service" }

// Kinds implements signal.Detector.
func (Service) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindService}
}

// Detect implements signal.Detector.
func (Service) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	out := loadBalancerPending(ctx, e)
	if text(e, kube.AttrSelector) == "" ||
		text(e, kube.AttrServiceType) == "ExternalName" {
		return out
	}
	return append(out, backendSignals(ctx, e)...)
}

// DefaultLoadBalancerPending is how long a LoadBalancer may wait for an
// address; cloud load balancers normally provision within minutes.
const DefaultLoadBalancerPending = 5 * time.Minute

func loadBalancerPending(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	if text(e, kube.AttrServiceType) != "LoadBalancer" ||
		flag(e, kube.AttrLoadBalancer) {
		return nil
	}
	since := valueSince(e, kube.AttrLoadBalancer)
	if !sustained(ctx, since, DefaultLoadBalancerPending) {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonLoadBalancerPending, Severity: signal.Warning,
		Since:   since,
		Summary: "LoadBalancer Service has no external address yet",
	}}
}

func backendSignals(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	if text(e, kube.AttrSelector) == "" ||
		text(e, kube.AttrServiceType) == "ExternalName" {
		return nil
	}
	slices := ctx.Model.Related(e.ID, knowledge.Backs, knowledge.Incoming)
	if len(slices) == 0 {
		return nil
	}
	endpoints, ready := 0.0, 0.0
	var since time.Time
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
	var out []signal.Signal
	if s, ok := portMismatch(ctx, e, slices); ok {
		out = append(out, s)
	}
	// No endpoints at all means nothing is selected (scaled to zero on
	// purpose); unready endpoints mean selected pods are failing.
	switch {
	case endpoints == 0:
		return out
	case ready == 0:
		if sustained(ctx, since, DefaultNoEndpoints) {
			out = append(out, signal.Signal{
				Reason:   constant.ReasonServiceNoEndpoints,
				Severity: signal.Critical, Since: since, Symptom: true,
				Summary: "Service has no ready backends; traffic to it " +
					"fails",
			})
		}
	case ready < endpoints:
		if sustained(ctx, since, DefaultDegradedBackends) {
			out = append(out, signal.Signal{
				Reason:   constant.ReasonServiceBackendsDegraded,
				Severity: signal.Warning, Since: since, Symptom: true,
				Summary: strconv.Itoa(int(ready)) + " of " +
					strconv.Itoa(int(endpoints)) +
					" Service backends are ready",
			})
		}
	}
	return out
}

// DefaultDegradedBackends is how long a Service may run with some unready
// backends; rolling updates pass through this state briefly.
const DefaultDegradedBackends = 5 * time.Minute

// portMismatch reports numeric target ports that no EndpointSlice
// publishes. Named target ports resolve per pod and are not checked.
func portMismatch(
	ctx signal.Context, e knowledge.Entity, slices []knowledge.EntityID,
) (signal.Signal, bool) {
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
		return signal.Signal{}, false
	}
	var missing []string
	for _, port := range strings.Split(text(e, kube.AttrTargetPorts), ",") {
		if _, err := strconv.Atoi(port); err == nil && !published[port] {
			missing = append(missing, port)
		}
	}
	if len(missing) == 0 {
		return signal.Signal{}, false
	}
	since := valueSince(e, kube.AttrTargetPorts)
	if !sustained(ctx, since, DefaultNoEndpoints) {
		return signal.Signal{}, false
	}
	return signal.Signal{
		Reason: constant.ReasonServicePortMismatch, Severity: signal.Warning,
		Since: since,
		Summary: "Service targets port " + strings.Join(missing, ", ") +
			", which its pods do not expose",
	}, true
}

// Ingress detects Ingresses whose backend Service does not exist.
type Ingress struct{}

// Name implements signal.Detector.
func (Ingress) Name() string { return "ingress" }

// Kinds implements signal.Detector.
func (Ingress) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindIngress}
}

// Detect implements signal.Detector.
func (Ingress) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	if !ctx.Synced(kube.KindService) {
		return nil
	}
	var missing []string
	for _, service := range ctx.Model.Related(
		e.ID, knowledge.RoutesTo, knowledge.Outgoing,
	) {
		if !ctx.Model.Exists(service) {
			missing = append(missing, service.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []signal.Signal{{
		Reason:   constant.ReasonIngressBackendNotFound,
		Severity: signal.Critical,
		Summary: "Ingress routes to Service " +
			strings.Join(missing, ", ") + ", which does not exist",
	}}
}

// EgressPolicy reports NetworkPolicies that block all outgoing traffic of
// the pods they select. This is often intended, so it is informational;
// the network-policy rule uses it as evidence when those pods fail.
type EgressPolicy struct{}

// Name implements signal.Detector.
func (EgressPolicy) Name() string { return "egress-policy" }

// Kinds implements signal.Detector.
func (EgressPolicy) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindNetworkPolicy}
}

// Detect implements signal.Detector.
func (EgressPolicy) Detect(
	_ signal.Context, e knowledge.Entity,
) []signal.Signal {
	if !flag(e, kube.AttrDeniesEgress) {
		return nil
	}
	return []signal.Signal{{
		Reason:   constant.ReasonRestrictiveNetworkPolicy,
		Severity: signal.Info,
		Since:    valueSince(e, kube.AttrDeniesEgress),
		Summary:  "Policy blocks all outgoing traffic of the pods it selects",
	}}
}
