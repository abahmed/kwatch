package explain

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A failing pod whose error names a Service of the cluster ("dial tcp
// redis:6379: connection refused") calls that Service. The words are
// only matched against the Services that exist and grouped by class
// (refused, unresolved, timed out); what the error means is never read.

// clusterCall is the in-cluster Service a failing pod's error names.
type clusterCall struct {
	service inventory.EntityID
	// port is the port the error names, 0 when it names none.
	port  int
	class detection.Mode
	// line is the error text the Service was read from, quoted as is.
	line string
}

// serviceMentions find host names in error text, most precise first:
// "host:port" anywhere, then a host after the words that dial or look
// one up. Group 1 is the host.
var serviceMentions = []*regexp.Regexp{
	regexp.MustCompile(`\b([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?):(\d{2,5})\b`),
	regexp.MustCompile(`(?:dial (?:tcp|udp)[46]?|lookup|connect to|` +
		`connecting to|://)\s?([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?)\b`),
}

// ModeBackendsFailing is a pod behind a Service, or the workload that
// owns such pods, that fails while the effect calls the Service. A finer
// mode names the Service: "BackendsFailing.shop/redis".
const ModeBackendsFailing detection.Mode = "BackendsFailing"

// ModeBackendChanged is a workload behind a Service with no ready
// endpoint that changed inside the causal window, such as one scaled to
// zero. A finer mode names the Service like ModeBackendsFailing.
const ModeBackendChanged detection.Mode = "BackendChanged"

// callMemo holds what a solve reads from error text and policies: the
// Service each failing pod names and the call each policy blocks.
type callMemo struct {
	clusterCalls map[inventory.EntityID]clusterCall
	blocks       map[blockKey]blockedCall
	owners       map[inventory.EntityID][]inventory.EntityID
}

// callModes are the pseudo modes of what a failing pod's call depends
// on: a policy that blocks it, the Service it names, the workloads
// behind that Service.
func (v *view) callModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	switch id.Kind {
	case kube.KindNetworkPolicy:
		return v.blockModes(id, effect, link)
	case kube.KindService:
		return v.calledServiceModes(id, effect, link)
	}
	return v.calledBackendModes(id, effect, link)
}

// serviceCallRows say that a Service with no ready endpoints, or the
// pods behind it, explain the pods that fail calling it.
var serviceCallRows = []Row{
	{
		// The Service the error names has nothing to answer with: its
		// pods are gone, not ready or scaled to zero.
		Name: "called-service-no-endpoints",
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{detection.ModeNoEndpoints}},
		Link:   LinkCalls,
		Effect: Side{Kind: kube.KindPod, Modes: callerFailures},
		Prior:  0.6,
	},
	{
		// The pods behind the Service the error names, or the workload
		// that owns them, fail: the workload is the one cause of both.
		Name: "called-service-backends-failing",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeBackendsFailing}},
		Link:   LinkBacks,
		Effect: Side{Kind: kube.KindPod, Modes: callerFailures},
		Prior:  0.7,
	},
	{
		// A workload behind the Service the error names was changed, such
		// as scaled to zero, and the Service has no ready endpoint left.
		Name: "called-service-backend-changed",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeBackendChanged}},
		Link:   LinkBacks,
		Effect: Side{Kind: kube.KindPod, Modes: callerFailures},
		Prior:  0.65,
	},
}

// serviceCallHops lead from a failing pod to the Service its error
// names, and to the workloads that own the pods behind that Service. The
// second hop skips the Service so a failing workload explains the caller
// directly.
func (v *view) serviceCallHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) {
		return nil
	}
	call, ok := v.clusterCallOf(pod)
	if !ok {
		return nil
	}
	out := []hop{{link: LinkCalls, to: call.service}}
	own := rootcause.TopOwner(v.s.Model, pod)
	for _, owner := range v.backendOwners(call.service) {
		if owner != own {
			out = append(out, hop{link: LinkBacks, to: owner})
		}
	}
	return out
}

// maxBackendWorkloads bounds the workloads checked for selecting one
// Service.
const maxBackendWorkloads = 256

// workloadKinds are the controllers whose pods a Service may select.
var workloadKinds = []inventory.Kind{
	kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet}

// backendOwners are the workloads behind a Service: the owners of the
// pods it selects and, when it has no ready endpoint, the workloads
// whose pod template it selects, which exist even when scaled to zero.
func (v *view) backendOwners(service inventory.EntityID) []inventory.EntityID {
	if cached, ok := v.owners[service]; ok {
		return cached
	}
	if v.owners == nil {
		v.owners = map[inventory.EntityID][]inventory.EntityID{}
	}
	out := v.ownersOfPods(service)
	if v.noReadyEndpoints(service) {
		out = append(out, v.selectingWorkloads(service, out)...)
	}
	v.owners[service] = out
	return out
}

// ownersOfPods are the owners of the pods the Service selects.
func (v *view) ownersOfPods(service inventory.EntityID) []inventory.EntityID {
	seen := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, backend := range sortedKeys(v.selected(service)) {
		owner := rootcause.TopOwner(v.s.Model, backend)
		if owner != backend && !seen[owner] {
			seen[owner] = true
			out = append(out, owner)
		}
	}
	return out
}

// selectingWorkloads are the workloads of the Service's namespace whose
// pod template it selects, other than the known ones.
func (v *view) selectingWorkloads(
	service inventory.EntityID, known []inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	checked := 0
	for _, kind := range workloadKinds {
		for _, id := range v.s.Model.EntitiesIn(kind, service.Namespace) {
			workload, ok := v.s.Model.Entity(id)
			if checked++; !ok || checked > maxBackendWorkloads ||
				containsID(known, id) {
				continue
			}
			if containsID(kube.ServicesSelecting(v.s.Model, workload),
				service) {
				out = append(out, id)
			}
		}
	}
	return out
}

// clusterCallOf is the Service the failing pod of id names in its
// errors.
func (v *view) clusterCallOf(id inventory.EntityID) (clusterCall, bool) {
	unit, ok := v.unitOf(id)
	if !ok {
		return clusterCall{}, false
	}
	if cached, ok := v.clusterCalls[unit]; ok {
		return cached, cached.line != ""
	}
	if v.clusterCalls == nil {
		v.clusterCalls = map[inventory.EntityID]clusterCall{}
	}
	var found clusterCall
	for _, f := range v.unitFindings(unit) {
		for _, e := range f.Evidence {
			if call, ok := v.serviceIn(e.Value, unit); ok {
				found = call
				break
			}
		}
		if found.line != "" {
			break
		}
	}
	v.clusterCalls[unit] = found
	return found, found.line != ""
}

// serviceIn finds the existing Service a failed call's text names.
// Every Service the text could name is a gate of the pod: one created
// later changes the walk.
func (v *view) serviceIn(
	text string, pod inventory.EntityID,
) (clusterCall, bool) {
	lower := strings.ToLower(text)
	class := endpointClass(lower)
	if class == "" || dnsServerFailure.MatchString(lower) {
		return clusterCall{}, false
	}
	for _, mention := range serviceMentions {
		for _, m := range mention.FindAllStringSubmatch(lower, -1) {
			call, ok := v.serviceNamed(m, pod)
			if ok {
				call.class, call.line = class, text
				return call, true
			}
		}
	}
	return clusterCall{}, false
}

// serviceNamed resolves one regexp match (host, optional port) to an
// existing Service; port 53 is a DNS query, never a Service call.
func (v *view) serviceNamed(
	m []string, pod inventory.EntityID,
) (clusterCall, bool) {
	ns, name, ok := kube.ClusterServiceName(m[1], pod.Namespace)
	if !ok || (len(m) > 2 && m[2] == "53") {
		return clusterCall{}, false
	}
	service := inventory.CoreID(kube.KindService, ns, name)
	v.gate(pod, service)
	if !v.s.Model.Exists(service) {
		return clusterCall{}, false
	}
	call := clusterCall{service: service}
	if len(m) > 2 {
		call.port, _ = strconv.Atoi(m[2])
	}
	return call, true
}

// calledServiceModes is the pseudo mode of a Service the effect's error
// names: no ready endpoints, read from its EndpointSlices.
func (v *view) calledServiceModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkCalls {
		return nil
	}
	call, ok := v.clusterCallOf(effect)
	if !ok || call.service != id || !v.noReadyEndpoints(id) ||
		v.backendChanged(id) {
		return nil
	}
	return []modeHealth{{mode: detection.ModeNoEndpoints,
		health: detection.Failing, pseudo: true}}
}

// backendChanged reports a workload behind the Service that changed
// inside the causal window: it, not the empty Service, is the cause.
func (v *view) backendChanged(service inventory.EntityID) bool {
	for _, owner := range v.backendOwners(service) {
		if len(v.changesOf(owner)) > 0 {
			return true
		}
	}
	return false
}

// noReadyEndpoints reports a Service with no ready endpoint at all. A
// Service whose slices were not seen yet is not known to have none.
func (v *view) noReadyEndpoints(service inventory.EntityID) bool {
	slices := rootcause.Reach(v.s.Model, service, inventory.Incoming, 1,
		inventory.Backs)
	if len(slices) == 0 {
		return v.s.synced(kube.KindEndpointSlice)
	}
	for _, slice := range slices {
		entity, ok := v.s.Model.Entity(slice.ID)
		if !ok {
			continue
		}
		attribute, ok := entity.Attribute(kube.AttrEndpointsReady)
		if ready, _ := attribute.Value.AsNumber(); ok && ready > 0 {
			return false
		}
	}
	return true
}

// calledBackendModes is the pseudo mode of a pod behind the Service the
// effect's error names, or of the workload owning such a pod, when that
// pod fails. It is read from the pods, not from the workload's own
// findings, which come later than its pods' failures.
func (v *view) calledBackendModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	call, ok := v.clusterCallOf(effect)
	if !ok || link != LinkBacks {
		return nil
	}
	named := detection.Mode("." + call.service.Namespace + "/" +
		call.service.Name)
	if id.Kind != kube.KindPod && len(v.changesOf(id)) > 0 &&
		v.noReadyEndpoints(call.service) {
		return []modeHealth{{mode: ModeBackendChanged + named,
			health: detection.Failing, pseudo: true}}
	}
	backends := v.selected(call.service)
	pods := []inventory.EntityID{id}
	if id.Kind != kube.KindPod {
		pods = rootcause.OwnedPods(v.s.Model, id)
	}
	for _, pod := range pods {
		if backends[pod] && v.unitFailing(pod) {
			return []modeHealth{{mode: ModeBackendsFailing + named,
				health: detection.Failing, pseudo: true}}
		}
	}
	return nil
}
