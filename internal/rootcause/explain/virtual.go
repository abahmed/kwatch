package explain

import (
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
	if id.Kind == kube.KindPod && v.dnsServer(id) {
		out = append(out, hop{link: LinkContains, to: kube.ClusterDNS})
	} else if id.Kind == kube.KindPod {
		out = append(out, hop{link: LinkResolvesVia, to: kube.ClusterDNS})
		out = append(out, v.policyCallHops(id)...)
		out = append(out, v.policyHops(id)...)
		out = append(out, v.schedulingHops(id)...)
		out = append(out, v.accessHops(id)...)
		out = append(out, v.removedNodeHops(id)...)
		out = append(out, v.helperHops(id)...)
		out = append(out, v.calledHops(id)...)
		out = append(out, v.serviceCallHops(id)...)
		out = append(out, v.missingServiceHops(id)...)
		out = append(out, v.agentHops(id)...)
		out = append(out, v.preemptionHops(id)...)
	}
	if id.Kind == kube.KindContainer {
		out = append(out, v.initWaitHops(id)...)
	}
	if v.hasMode(id, createModes) {
		out = append(out, v.admitHops(id)...)
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

// dnsServer reports whether a pod serves the cluster DNS: it belongs
// to the coredns or kube-dns Deployment of kube-system.
func (v *view) dnsServer(pod inventory.EntityID) bool {
	if pod.Namespace != "kube-system" {
		return false
	}
	top := rootcause.TopOwner(v.s.Model, pod)
	return top != pod && kube.DNSServerNames[top.Name]
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

// admitHops lead from a controller that cannot create objects to the
// webhooks and quotas that may be rejecting the creates.
func (v *view) admitHops(id inventory.EntityID) []hop {
	var out []hop
	for _, kind := range admitKinds {
		for _, hook := range v.s.Model.Entities(kind) {
			out = append(out, hop{link: LinkAdmits, to: hook})
		}
	}
	if id.Namespace == "" {
		return out
	}
	namespace := inventory.CoreID(kube.KindNamespace, "", id.Namespace)
	for _, quota := range v.s.Model.Related(
		namespace, inventory.Constrains, inventory.Incoming,
	) {
		out = append(out, hop{link: LinkAdmits, to: quota})
	}
	return out
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
	var out []hop
	for _, pod := range sortedKeys(v.selected(service)) {
		out = append(out, hop{link: LinkBacks, to: pod})
	}
	return out
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
