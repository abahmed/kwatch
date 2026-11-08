package investigate

import (
	"context"
	"strconv"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// The checks kwatch can run to settle a close call, each read-only and
// bounded (kube.CloseCall): a DNS lookup of the endpoint's name and one
// TCP connect to its address. Others a reader might expect are not run:
// kwatch makes no readiness call to a workload and does not pull an
// image to see whether it can.

// closeCallCheck settles a close call between an external endpoint and
// another cause by looking at the endpoint once. It adds its finding to
// the evidence and leads nothing.
func closeCallCheck() check {
	return check{name: incident.CheckCloseCall, title: "Check",
		budget: apiReadBudget, applies: closeCallApplies, adds: true,
		summary: readCloseCall}
}

// closeCallEndpoint returns the external endpoint among the two causes
// of a close call, and the other cause.
func closeCallEndpoint(
	p incident.Incident,
) (endpoint inventory.EntityID, other *rootcause.CauseRecord, ok bool) {
	cause := p.Cause
	if cause == nil || cause.Rival == nil {
		return endpoint, nil, false
	}
	switch {
	case cause.Root.Kind == kube.KindExternalEndpoint:
		return cause.Root, cause.Rival, true
	case cause.Rival.Root.Kind == kube.KindExternalEndpoint:
		return cause.Rival.Root, cause, true
	}
	return endpoint, nil, false
}

func closeCallApplies(p incident.Incident, _ inventory.Reader) bool {
	_, _, ok := closeCallEndpoint(p)
	return ok
}

// readCloseCall runs the check and words what it found, as a fact.
func readCloseCall(
	ctx context.Context, s Sources, p incident.Incident,
) Result {
	endpoint, _, ok := closeCallEndpoint(p)
	if !ok || s.CloseCall == nil {
		return Result{}
	}
	text := closeCallWords(s.CloseCall(ctx, endpoint.Name))
	if text == "" {
		return Result{}
	}
	return Result{Evidence: []incident.Fact{{Kind: incident.FactCheck,
		Subject: endpoint.Name, Text: text}}}
}

// closeCallWords says what the check found and, when it is a verdict,
// what it rules out. An inconclusive check states the fact and no
// verdict.
func closeCallWords(r kube.CloseCallResult) string {
	switch {
	case r.Blocked || r.Host == "":
		return ""
	case r.TimedOut:
		return "kwatch checked " + r.Endpoint + ": no answer in " +
			strconv.Itoa(int(kube.CloseCallTimeout.Seconds())) +
			" seconds."
	case !r.Resolved:
		return "kwatch checked: " + r.Host + " does not resolve."
	case r.Connected:
		return "kwatch checked: " + r.Host + " resolves from kwatch and " +
			"accepts a connection from kwatch."
	case r.Refused:
		return "kwatch checked: " + r.Host + " resolves from kwatch but " +
			"refuses connections."
	}
	return "kwatch checked: " + r.Host + " resolves from kwatch but a " +
		"connection from kwatch failed: " + r.Error + "."
}
