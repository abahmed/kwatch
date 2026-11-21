package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkPeers leads from a pod that waits for its peers to the headless
// Service whose names it looks up.
const LinkPeers LinkType = "peers"

// peerName reads a peer's stable name from error text in lower case:
// "<pod>.<service>.<namespace>.svc". Group 1 is the pod, 2 the Service
// and 3 the namespace.
var peerName = regexp.MustCompile(
	`([a-z0-9][a-z0-9-]*)\.([a-z0-9][a-z0-9-]*)\.([a-z0-9][a-z0-9-]*)\.svc`)

// peerRows say that a headless Service that publishes only ready
// addresses explains the pods that wait for each other's names: a pod
// is not in the answer until it is ready, and is not ready until it
// finds its peers.
var peerRows = []Row{
	{
		Name: "headless-hides-not-ready-peers",
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{ModeHidesNotReady}},
		Link: LinkPeers,
		Effect: Side{Kind: kube.KindPod, Modes: callerFailuresAnd(
			detection.ModeNotReady, detection.ModeProbeReadiness)},
		Prior: 0.8, MinCovered: 2,
	},
}

// peerServices are the headless Services of the pod's namespace that
// publish only ready addresses and whose pod names the pod's errors
// look up.
func (v *view) peerServices(pod inventory.EntityID) []inventory.EntityID {
	var out []inventory.EntityID
	for _, line := range v.errorLines(pod) {
		for _, m := range peerName.FindAllStringSubmatch(
			strings.ToLower(line), -1) {
			if m[3] != pod.Namespace {
				continue
			}
			svc := inventory.CoreID(kube.KindService, m[3], m[2])
			if v.hidesNotReady(svc) && !containsID(out, svc) {
				out = append(out, svc)
			}
		}
	}
	sortIDs(out)
	return out
}

// hidesNotReady reports a headless Service that leaves the addresses of
// pods that are not ready out of its answers.
func (v *view) hidesNotReady(svc inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(svc)
	return ok && attrBool(e, kube.AttrHeadless) &&
		!attrBool(e, kube.AttrPublishNotReady)
}

// peerHops lead from a pod that is not ready to the headless Services
// whose peer names its errors look up.
func (v *view) peerHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) || !v.s.synced(kube.KindService) {
		return nil
	}
	services := v.peerServices(pod)
	for _, svc := range services {
		v.gate(pod, svc)
	}
	return hopsTo(LinkPeers, services...)
}

// peerModes is ModeHidesNotReady for a headless Service the failing
// effect looks up peer names of.
func (v *view) peerModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkPeers {
		return nil
	}
	unit, ok := v.unitOf(effect)
	if !ok || !containsID(v.peerServices(unit), id) {
		return nil
	}
	return pseudoFailing(ModeHidesNotReady)
}
