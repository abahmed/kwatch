package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// admitKinds are the admission webhooks every create passes through.
var admitKinds = []inventory.Kind{
	kube.KindValidatingHook, kube.KindMutatingWebhook,
}

// createModes are the modes of a controller that cannot create objects.
var createModes = []detection.Mode{
	detection.ModeFailedCreate, detection.ModeReplicaFailure}

// metricsAPIMarker names the APIServices that serve autoscaling metrics:
// metrics.k8s.io, custom.metrics.k8s.io and external.metrics.k8s.io.
const metricsAPIMarker = "metrics.k8s.io"

// virtualHops are dependencies no relation stores: shared cluster
// services, admission and scheduling. Each is added only when the
// entity shows a failure that such a dependency can explain, so healthy
// objects never fan out to every webhook in the cluster.
func (v *view) virtualHops(id inventory.EntityID) []hop {
	var out []hop
	out = append(out, v.dnsWorkloadHops(id)...)
	if id.Kind == kube.KindPod && v.dnsServer(id) {
		out = append(out, hop{link: LinkContains, to: kube.ClusterDNS})
	} else if id.Kind == kube.KindPod {
		out = append(out, hop{link: LinkResolvesVia, to: kube.ClusterDNS})
		out = append(out, v.policyCallHops(id)...)
		out = append(out, v.policyHops(id)...)
		out = append(out, v.schedulingHops(id)...)
		out = append(out, v.claimHops(id)...)
		out = append(out, v.holdHops(id)...)
		out = append(out, v.accessHops(id)...)
		out = append(out, v.removedNodeHops(id)...)
		out = append(out, v.helperHops(id)...)
		out = append(out, v.calledHops(id)...)
		out = append(out, v.serviceCallHops(id)...)
		out = append(out, v.missingServiceHops(id)...)
		out = append(out, v.agentHops(id)...)
		out = append(out, v.preemptionHops(id)...)
	}
	if id.Kind == kube.KindPVC {
		out = append(out, v.provisionerHops(id)...)
	}
	if id.Kind == kube.KindContainer {
		out = append(out, v.initWaitHops(id)...)
	}
	if v.hasMode(id, createModes) {
		out = append(out, v.allAdmitHops(id)...)
	}
	if id.Kind == kube.KindService {
		out = append(out, v.backendHops(id)...)
		out = append(out, v.webhookHops(id)...)
	}
	if id.Kind == kube.KindNode {
		out = append(out, v.drainBlockerHops(id)...)
	}
	out = append(out, v.scalerHops(id)...)
	out = append(out, v.finalizerHops(id)...)
	if id.Kind == kube.KindHPA && len(v.s.Findings[id]) > 0 {
		out = append(out, v.metricsAPIHops()...)
	}
	if len(v.s.Findings[id]) > 0 {
		v.gate(id, kube.APIServer)
		if len(v.s.Findings[kube.APIServer]) > 0 {
			out = append(out, hop{link: LinkServedBy,
				to: kube.APIServer})
		}
	}
	return append(out, v.controlPlaneHops(id)...)
}

// dnsSignalled reports whether any failure's error text shows name
// resolution failing. It is read for the DNS servers themselves, whose
// own errors rarely say so.
func (v *view) dnsSignalled() bool {
	if v.dnsSeen == nil {
		seen := false
		for id, found := range v.s.Findings {
			if len(found) > 0 && hasSignal(SignalDNS, v.text(id)) {
				seen = true
				break
			}
		}
		v.dnsSeen = &seen
	}
	return *v.dnsSeen
}

// resolutionFailing reports whether the cluster DNS fails as seen from
// effect: its error shows name resolution failing, or, for the DNS
// servers themselves, anyone's does.
func (v *view) resolutionFailing(
	effect inventory.EntityID, link LinkType,
) bool {
	if link == LinkResolvesVia && v.unknownExternalName(effect) {
		return false
	}
	return hasSignal(SignalDNS, v.text(effect)) ||
		(link == LinkContains && v.dnsSignalled())
}

// unknownExternalName reports whether the effect's error is the DNS
// answering that a name outside the cluster does not exist ("lookup
// api.typo.example: no such host"): the name's problem, not the
// cluster DNS's.
func (v *view) unknownExternalName(effect inventory.EntityID) bool {
	call, ok := endpointIn(v.text(effect))
	if !ok || call.class != endpointUnresolved {
		return false
	}
	host := endpointHost(call.endpoint)
	unit, ok := v.unitOf(effect)
	return ok && strings.Contains(host, ".") && !v.inCluster(host, unit)
}

// registryOf is the virtual registry entity an image is pulled from.
func registryOf(image inventory.EntityID) inventory.EntityID {
	return inventory.CoreID(kube.KindRegistry, "",
		rootcause.RegistryHost(image.Name))
}

// podOf returns the pod of a container.
func (v *view) podOf(id inventory.EntityID) (inventory.EntityID, bool) {
	return rootcause.PodOf(v.s.Model, id)
}

// hasMode reports whether one of id's findings matches one of modes.
func (v *view) hasMode(
	id inventory.EntityID, modes []detection.Mode,
) bool {
	for _, f := range v.s.Findings[id] {
		if anyModeMatches(modes, f.Mode) {
			return true
		}
	}
	return false
}

// webhookAdmitHops lead from a controller that cannot create objects to
// the webhooks that may be rejecting the creates.
func (v *view) webhookAdmitHops(_ inventory.EntityID) []hop {
	var out []hop
	for _, kind := range admitKinds {
		out = append(out, hopsTo(LinkAdmits, v.s.Model.Entities(kind)...)...)
	}
	return out
}

// quotaAdmitHops lead from a controller that cannot create objects to
// the quotas of its namespace.
func (v *view) quotaAdmitHops(id inventory.EntityID) []hop {
	if id.Namespace == "" {
		return nil
	}
	namespace := inventory.CoreID(kube.KindNamespace, "", id.Namespace)
	return hopsTo(LinkAdmits, v.s.Model.Related(
		namespace, inventory.Constrains, inventory.Incoming)...)
}

// webhookHops lead from a Service to the admission webhooks it serves.
func (v *view) webhookHops(service inventory.EntityID) []hop {
	var out []hop
	for _, hook := range v.s.Model.Related(
		service, inventory.Serves, inventory.Incoming,
	) {
		if hook.Kind == kube.KindValidatingHook ||
			hook.Kind == kube.KindMutatingWebhook {
			out = append(out, hop{link: LinkContains, to: hook})
		}
	}
	return out
}

// metricsAPIHops lead from an autoscaler to the metrics APIServices.
func (v *view) metricsAPIHops() []hop {
	var out []hop
	for _, api := range v.s.Model.Entities(kube.KindAPIService) {
		if strings.Contains(api.Name, metricsAPIMarker) {
			out = append(out, hop{link: LinkServedBy, to: api})
		}
	}
	return out
}

// schedulingHops lead from an unschedulable pod to the constraint that
// rejects most nodes, as a virtual "scheduling" entity.
func (v *view) schedulingHops(pod inventory.EntityID) []hop {
	reason, ok := v.schedulerVerdict(pod)
	switch {
	case !ok:
		return nil
	case v.capacityLeft(reason, pod):
		// The shortage is the node that left, not a reason of its
		// own: the removed node is the candidate.
		return nil
	case volumeConflict(reason) && len(v.claimsOf(pod)) > 0:
		// The claim, not the scheduler, pins the pod; its mount
		// relation is the hop (storedHops).
		return nil
	}
	return []hop{{link: LinkSchedules, to: inventory.CoreID(
		KindScheduling, "", reason)}}
}

// schedulerVerdict is the most common reason the scheduler gave for
// not placing the pod, read from its findings' evidence.
func (v *view) schedulerVerdict(pod inventory.EntityID) (string, bool) {
	for _, f := range v.s.Findings[pod] {
		for _, e := range f.Evidence {
			if e.Label != "scheduler" {
				continue
			}
			if blockers, _ := kube.ParseSchedulerMessage(
				e.Value); len(blockers) > 0 {
				return blockers[0].Reason, true
			}
		}
	}
	return "", false
}

// policyHops lead from a pod to the network policies selecting it.
func (v *view) policyHops(pod inventory.EntityID) []hop {
	var out []hop
	for _, policy := range v.policiesIn(pod.Namespace) {
		if v.selected(policy)[pod] {
			out = append(out, hop{link: LinkRestricts, to: policy})
		}
	}
	return out
}

// backendHops lead from a Service to the pods it selects.
func (v *view) backendHops(service inventory.EntityID) []hop {
	return hopsTo(LinkBacks, sortedKeys(v.selected(service))...)
}

// servicesSelecting lists the Services of the pod's namespace that
// select it.
func (v *view) servicesSelecting(pod inventory.EntityID) []inventory.EntityID {
	var out []inventory.EntityID
	for _, service := range v.s.Model.Entities(kube.KindService) {
		if service.Namespace == pod.Namespace && v.selected(service)[pod] {
			out = append(out, service)
		}
	}
	return out
}

// policiesIn lists the network policies of a namespace.
func (v *view) policiesIn(namespace string) []inventory.EntityID {
	var out []inventory.EntityID
	for _, policy := range v.s.Model.Entities(kube.KindNetworkPolicy) {
		if policy.Namespace == namespace {
			out = append(out, policy)
		}
	}
	return out
}

// selected returns the pods an object selects, resolved once.
func (v *view) selected(
	id inventory.EntityID,
) map[inventory.EntityID]bool {
	if cached, ok := v.selects[id]; ok {
		return cached
	}
	out := map[inventory.EntityID]bool{}
	if v.s.Links != nil {
		for _, link := range v.s.Links.Links(id) {
			if link.Type == inventory.Selects {
				out[link.To] = true
			}
		}
	}
	v.selects[id] = out
	return out
}

// volumeAffinityConflict is the scheduler's wording for a pod whose
// bound volume lives where the pod cannot run.
const volumeAffinityConflict = "volume node affinity conflict"

// volumeConflict reports a scheduler reason naming a volume node
// affinity conflict.
func volumeConflict(reason string) bool {
	return strings.Contains(reason, volumeAffinityConflict)
}

// claimsOf lists the claims a pod mounts.
func (v *view) claimsOf(pod inventory.EntityID) []inventory.EntityID {
	return v.s.Model.Related(pod, inventory.Mounts, inventory.Outgoing)
}

// pinnedClaim reports whether claim pins the effect pod: the scheduler
// rejected the pod for a volume node affinity conflict and the pod
// mounts the claim. It is the pseudo mode of the claim for that pod.
func (v *view) pinnedClaim(claim, effect inventory.EntityID) bool {
	pod, ok := v.unitOf(effect)
	if !ok {
		return false
	}
	reason, ok := v.schedulerVerdict(pod)
	return ok && volumeConflict(reason) && containsID(v.claimsOf(pod), claim)
}

// The virtual_*.go files each let a piece of text or an attribute name
// an object: a webhook, a quota, a Service, an address, a finalizer. They
// share one shape: extract a name from the text, resolve it to an entity
// of the model (or a virtual one), then add a hop to it and report a
// pseudo mode once the effect's own evidence confirms it. These helpers
// are the shared pieces of that shape.

// pseudoFailing is the single pseudo mode a virtual or absent entity
// reports while it is the cause of the effect.
func pseudoFailing(mode detection.Mode) []modeHealth {
	return []modeHealth{{mode: mode, health: detection.Failing,
		pseudo: true}}
}

// hopsTo is one hop of the link to each of the entities.
func hopsTo(link LinkType, ids ...inventory.EntityID) []hop {
	var out []hop
	for _, id := range ids {
		out = append(out, hop{link: link, to: id})
	}
	return out
}

// namesIn is the text each match of the patterns names: group 1 of
// every match, patterns in order. The text is lower case.
func namesIn(text string, patterns ...*regexp.Regexp) []string {
	var out []string
	for _, pattern := range patterns {
		for _, m := range pattern.FindAllStringSubmatch(text, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// firstNameIn is the first name any pattern finds, in pattern order, or
// "".
func firstNameIn(text string, patterns ...*regexp.Regexp) string {
	for _, pattern := range patterns {
		if m := pattern.FindStringSubmatch(text); m != nil {
			return m[1]
		}
	}
	return ""
}

// namesObject reports whether the text names the object, ignoring case.
func namesObject(text, name string, patterns ...*regexp.Regexp) bool {
	for _, named := range namesIn(strings.ToLower(text), patterns...) {
		if strings.EqualFold(named, name) {
			return true
		}
	}
	return false
}

// attrList is a comma-separated attribute of an entity as a list.
func attrList(e inventory.Entity, name string) []string {
	return strings.Split(attrText(e, name), ",")
}

// clusterServiceID resolves a host a pod's text names ("redis",
// "redis.shop", "redis.shop.svc.cluster.local") to the Service it would
// reach from the pod's namespace, whether or not that Service exists.
func clusterServiceID(
	host string, from inventory.EntityID,
) (inventory.EntityID, bool) {
	ns, name, ok := kube.ClusterServiceName(host, from.Namespace)
	if !ok {
		return inventory.EntityID{}, false
	}
	return inventory.CoreID(kube.KindService, ns, name), true
}
