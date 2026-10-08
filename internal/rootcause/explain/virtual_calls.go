package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Links read from error text, not from stored relations.
const (
	// LinkCalls: the effect calls the cause over the network, as its
	// error text says ("dial tcp db.example.com:5432").
	LinkCalls LinkType = "calls"
	// LinkSharesError: the effect fails with the error the cause
	// stands for.
	LinkSharesError LinkType = "shares-error"
)

// Virtual kinds read from error text. They let a storm with no
// Kubernetes object to blame still become one incident.
const (
	// KindExternalEndpoint is a network endpoint outside the cluster
	// objects kwatch watches, such as "db.example.com:5432", named in
	// error text or in a pod's configuration (see kube.KindExternalEndpoint).
	KindExternalEndpoint = kube.KindExternalEndpoint
	// KindFailureSignature is one error message several workloads
	// fail with, normalised so IDs and numbers do not split it. Its
	// name is built by signatureName.
	KindFailureSignature inventory.Kind = "failure-signature"
)

// Pseudo modes of the virtual kinds above.
const (
	// ModeEndpointFailing is an endpoint its callers cannot use. A
	// finer mode says how: "EndpointFailing.Refused".
	ModeEndpointFailing detection.Mode = "EndpointFailing"
	// ModeSharedSignature is an error several workloads share.
	ModeSharedSignature detection.Mode = "SharedSignature"
)

// callerFailures are the modes of a workload that fails because a
// call failed: it crashes or exits. Probe and pull failures are left
// out: the probe is the kubelet's call, and pulls have the registry.
var callerFailures = []detection.Mode{
	detection.ModeCrashLoop, detection.ModeRestarting, detection.ModeExit,
	detection.ModeFailed, detection.ModeError}

// calledRows cover failures that only error text connects: many
// workloads failing on one external endpoint, or with one error.
var calledRows = []Row{
	{
		// A dependency a pod is configured to call, which kwatch's own
		// probe finds refusing connections. The probe is the proof, so
		// the callers' errors need not name it, and one workload is
		// enough: the endpoint is down whoever calls it.
		Name: "external-endpoint-unreachable",
		Cause: Side{Kind: KindExternalEndpoint,
			Modes: []detection.Mode{detection.ModeActiveProbe}},
		Link:   LinkCalls,
		Effect: Side{Kind: kube.KindPod, Modes: callerFailures},
		Prior:  0.75,
	},
	{
		Name: "external-endpoint-failing",
		Cause: Side{Kind: KindExternalEndpoint,
			Modes: []detection.Mode{ModeEndpointFailing}},
		Link: LinkCalls,
		Effect: Side{Kind: kube.KindPod, Modes: callerFailures,
			Signal: SignalConnection},
		// Under the cluster DNS rows: when name resolution fails for
		// everyone, the DNS outage is the better cause.
		Prior: 0.65, MinWorkloads: EndpointMinWorkloads,
	},
	{
		// The weakest row: the words match, nothing names a cause.
		// It still makes one incident of a storm instead of one per
		// workload.
		Name: "shared-failure-signature",
		Cause: Side{Kind: KindFailureSignature,
			Modes: []detection.Mode{ModeSharedSignature}},
		Link:   LinkSharesError,
		Effect: podSide(callerFailures...),
		Prior:  0.55, MinWorkloads: SignatureMinWorkloads,
	},
}

// calledHops lead from a failing pod to the endpoint its error names,
// or else to the error signature it shares with others.
func (v *view) calledHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) {
		return nil
	}
	// Both hops are offered. Why: an endpoint only counts when
	// EndpointMinWorkloads workloads name the same one, and pods that
	// each name a different address (a pod IP, a changing port) would
	// otherwise never reach the signature they all share. The rows'
	// priors and minimums decide which one stands.
	var hops []hop
	if call, ok := v.callOf(pod); ok {
		hops = append(hops, hop{link: LinkCalls, to: inventory.CoreID(
			KindExternalEndpoint, "", call.endpoint)})
	}
	if name, ok := v.signatureOf(pod); ok {
		hops = append(hops, hop{link: LinkSharesError, to: inventory.CoreID(
			KindFailureSignature, "", name)})
	}
	return hops
}

// calledModes are the pseudo modes of an endpoint or a signature, read
// from the effect: it fails only for the effects that name it.
func (v *view) calledModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	failing := func(mode detection.Mode) []modeHealth {
		return []modeHealth{{mode: mode, health: detection.Failing,
			pseudo: true}}
	}
	switch {
	case id.Kind == KindExternalEndpoint && link == LinkCalls:
		if call, ok := v.callOf(effect); ok && call.endpoint == id.Name {
			return failing(ModeEndpointFailing + "." + call.class)
		}
	case id.Kind == KindFailureSignature && link == LinkSharesError:
		if name, ok := v.signatureOf(effect); ok && name == id.Name {
			return failing(ModeSharedSignature)
		}
	}
	return nil
}

// probeEvidenceLabel labels the kubelet's own probe failure message. Its
// address is the kubelet's call to the pod, never one the pod makes.
const probeEvidenceLabel = "probe"

// callOf is the endpoint the failing pod of id names in its errors.
func (v *view) callOf(id inventory.EntityID) (endpointCall, bool) {
	unit, ok := v.unitOf(id)
	if !ok {
		return endpointCall{}, false
	}
	if cached, ok := v.calls[unit]; ok {
		return cached, cached.endpoint != ""
	}
	var found endpointCall
	for _, f := range v.unitFindings(unit) {
		for _, e := range f.Evidence {
			if e.Label == probeEvidenceLabel {
				continue
			}
			call, ok := endpointIn(e.Value)
			if ok && !v.inCluster(endpointHost(call.endpoint), unit) {
				found = call
				break
			}
		}
		if found.endpoint != "" {
			break
		}
	}
	v.calls[unit] = found
	return found, found.endpoint != ""
}

// signatureOf is the error signature of the failing pod of id, read
// from its containers' own error messages (the "error" evidence: the
// last termination message, or, when that is empty, the first error line
// of the previous log, see detectors/container.go), never from event
// text every crash shares. The text is only normalised, never
// interpreted.
func (v *view) signatureOf(id inventory.EntityID) (string, bool) {
	unit, ok := v.unitOf(id)
	if !ok {
		return "", false
	}
	if cached, ok := v.signatures[unit]; ok {
		return cached, cached != ""
	}
	name := ""
	for _, f := range v.unitFindings(unit) {
		for _, e := range f.Evidence {
			if e.Label != "error" {
				continue
			}
			if signature := normalizeSignature(e.Value); signature != "" {
				name = signatureName(f.Mode, signature)
				break
			}
		}
		if name != "" {
			break
		}
	}
	v.signatures[unit] = name
	return name, name != ""
}

// unitOf is the pod that fails for id: the pod itself, or a
// container's pod.
func (v *view) unitOf(id inventory.EntityID) (inventory.EntityID, bool) {
	switch id.Kind {
	case kube.KindPod:
		return id, true
	case kube.KindContainer:
		return v.podOf(id)
	}
	return inventory.EntityID{}, false
}

// unitFindings are the unhealthy findings of a pod and its containers
// that a failed call can explain, in a stable order.
func (v *view) unitFindings(pod inventory.EntityID) []detection.Finding {
	ids := append([]inventory.EntityID{pod}, v.s.Model.Related(
		pod, inventory.PartOf, inventory.Incoming)...)
	sortIDs(ids)
	var out []detection.Finding
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			if unhealthy(f) && anyModeMatches(callerFailures, f.Mode) {
				out = append(out, f)
			}
		}
	}
	return out
}

// inCluster reports whether host is the pod itself or a Service of the
// cluster: "localhost", "db.shop.svc", or "db" when the pod's
// namespace has a Service db.
func (v *view) inCluster(host string, pod inventory.EntityID) bool {
	if clusterLocal(host) {
		return true
	}
	name, namespace, qualified := cutLabel(host)
	if !qualified {
		namespace = pod.Namespace
	}
	return v.s.Model.Exists(inventory.CoreID(kube.KindService, namespace,
		name))
}

// cutLabel splits "db.shop" into the Service name and namespace. A
// host with more labels is a domain name, never a Service.
func cutLabel(host string) (name, namespace string, qualified bool) {
	name, namespace, qualified = strings.Cut(host, ".")
	if qualified && strings.Contains(namespace, ".") {
		return host, "", true
	}
	return name, namespace, qualified
}
