package detectors

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxFitNodes bounds the nodes checked for one pod, so a very large
// cluster costs a bounded amount per pending pod.
const maxFitNodes = 200

// Resource keys of the fit check. Anything else is an extended resource
// such as nvidia.com/gpu and is keyed by its own name.
const (
	resCPU       = "cpu"
	resMemory    = "memory"
	resEphemeral = "ephemeral-storage"
	resPods      = "pods"
)

// fitNode is a node as the scheduler sees it for one pending pod.
type fitNode struct {
	id       inventory.EntityID
	group    string
	labels   map[string]string
	labelsOK bool
	taints   []kube.Taint
	free     map[string]float64
	cordoned bool
	notReady bool
}

// fitPeer is a pod that runs on a node, for (anti-)affinity and spread.
type fitPeer struct {
	namespace, name string
	labels          map[string]string
}

// fitClaim is a volume claim of the pod and the node terms it limits
// the pod to: empty when the claim does not limit placement.
type fitClaim struct {
	name string
	// volume is the bound PersistentVolume's name; empty for a claim
	// that is not bound yet.
	volume string
	terms  [][]kube.Requirement
}

// fitCase is everything known about the pending pod's placement.
type fitCase struct {
	pod    inventory.EntityID
	spec   kube.SchedulingSpec
	labels map[string]string
	need   map[string]float64
	claims []fitClaim
	nodes  []fitNode
	peers  map[string][]fitPeer
	total  int
}

// fitEvidence explains an unschedulable pod beyond the scheduler's own
// message: which node pools could take it, what stops each, and what
// the autoscaler says. It checks every constraint kwatch can read
// (resources, taints, node selector and affinity, volume topology, pod
// affinity, topology spread) and is best-effort for the last two.
func fitEvidence(
	ctx detection.Context, pod inventory.Entity,
) []detection.Evidence {
	c := newFitCase(ctx.Model, pod)
	out := c.verdict()
	return append(out, autoscalerEvidence(ctx, pod.ID)...)
}

func newFitCase(model inventory.Reader, pod inventory.Entity) *fitCase {
	c := &fitCase{pod: pod.ID, spec: kube.ParseSchedulingSpec(pod),
		labels: kube.ParseLabels(text(pod, kube.AttrLabels)),
		need:   podNeeds(model, pod.ID), peers: map[string][]fitPeer{}}
	c.claims = podClaimTerms(model, pod.ID)
	ids := model.Entities(kube.KindNode)
	sort.Slice(ids, func(i, j int) bool { return ids[i].Name < ids[j].Name })
	c.total = len(ids)
	for _, id := range ids[:min(len(ids), maxFitNodes)] {
		node, ok := model.Entity(id)
		if !ok {
			continue
		}
		c.nodes = append(c.nodes, newFitNode(model, node, c.need))
		c.peers[id.Name] = peersOn(model, id)
	}
	return c
}

func newFitNode(
	model inventory.Reader, node inventory.Entity, need map[string]float64,
) fitNode {
	zone, group := "", ""
	for _, id := range model.Related(node.ID, inventory.PartOf,
		inventory.Outgoing) {
		switch id.Kind {
		case kube.KindZone:
			zone = id.Name
		case kube.KindNodePool:
			group = id.Name
		}
	}
	if group == "" {
		group = text(node, kube.AttrInstanceType)
	}
	labels, ok := kube.NodeLabels(node, zone)
	n := fitNode{id: node.ID, group: group, labels: labels, labelsOK: ok,
		taints:   kube.ParseTaints(text(node, kube.AttrTaints)),
		cordoned: flag(node, kube.AttrUnschedulable)}
	if _, known := node.Attribute(kube.AttrReady); known {
		n.notReady = !flag(node, kube.AttrReady)
	}
	n.free = nodeFree(model, node, need)
	return n
}

// nodeFree is what each needed resource has left on the node. A
// resource whose capacity kwatch does not know is left out and never
// blocks.
func nodeFree(
	model inventory.Reader, node inventory.Entity, need map[string]float64,
) map[string]float64 {
	extra := kube.ParseResourceList(text(node, kube.AttrAllocatable))
	free := map[string]float64{}
	for key := range need {
		total, known := allocatable(node, extra, key)
		if !known {
			continue
		}
		free[key] = total - requestedFit(model, node.ID, key)
	}
	return free
}

func allocatable(
	node inventory.Entity, extra map[string]float64, key string,
) (float64, bool) {
	switch key {
	case resCPU:
		return number(node, kube.AttrCPUAllocatable)
	case resMemory:
		return number(node, kube.AttrMemoryAllocatable)
	case resEphemeral, resPods:
		total, ok := extra[key]
		return total, ok
	}
	// A node without the extended resource has none of it.
	return extra[key], true
}

// requestedFit sums what the running pods of a node hold of a resource.
func requestedFit(
	model inventory.Reader, node inventory.EntityID, key string,
) float64 {
	total := 0.0
	for _, id := range model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		if pod, ok := model.Entity(id); ok && podFinished(pod) {
			continue
		}
		if key == resPods {
			total++
		} else {
			total += podAsk(model, id, key)
		}
	}
	return total
}

// podNeeds is what the pod asks of each resource, plus one pod slot.
func podNeeds(
	model inventory.Reader, pod inventory.EntityID,
) map[string]float64 {
	keys := map[string]bool{resCPU: true, resMemory: true,
		resEphemeral: true}
	for _, id := range model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		if c, ok := model.Entity(id); ok {
			for name := range kube.ParseResourceList(
				text(c, kube.AttrExtendedReq)) {
				keys[name] = true
			}
		}
	}
	need := map[string]float64{resPods: 1}
	for key := range keys {
		if v := podAsk(model, pod, key); v > 0 {
			need[key] = v
		}
	}
	return need
}

// podAsk is what the scheduler reserves of one resource for a pod: the
// sum over its containers, or its largest init container when bigger.
func podAsk(
	model inventory.Reader, pod inventory.EntityID, key string,
) float64 {
	sum, initMax := 0.0, 0.0
	for _, id := range model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		c, ok := model.Entity(id)
		if !ok {
			continue
		}
		v := containerAsk(c, key)
		if flag(c, kube.AttrInit) {
			initMax = max(initMax, v)
		} else {
			sum += v
		}
	}
	return max(sum, initMax)
}

func containerAsk(c inventory.Entity, key string) float64 {
	attr := ""
	switch key {
	case resCPU:
		attr = kube.AttrCPUReq
	case resMemory:
		attr = kube.AttrMemoryReq
	case resEphemeral:
		attr = kube.AttrEphemeralReq
	default:
		return kube.ParseResourceList(text(c, kube.AttrExtendedReq))[key]
	}
	v, _ := number(c, attr)
	return v
}

// peersOn lists the pods that still run on a node.
func peersOn(model inventory.Reader, node inventory.EntityID) []fitPeer {
	var out []fitPeer
	for _, id := range model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		pod, ok := model.Entity(id)
		if !ok || podFinished(pod) {
			continue
		}
		out = append(out, fitPeer{namespace: id.Namespace, name: id.Name,
			labels: kube.ParseLabels(text(pod, kube.AttrLabels))})
	}
	return out
}

// podClaimTerms reads the placement limits of the pod's volume claims:
// the bound volume's node affinity, or for a claim not yet bound the
// storage class's allowed topology.
func podClaimTerms(
	model inventory.Reader, pod inventory.EntityID,
) []fitClaim {
	var out []fitClaim
	for _, id := range model.Related(pod, inventory.Mounts,
		inventory.Outgoing) {
		if id.Kind != kube.KindPVC {
			continue
		}
		if terms, volume := claimTerms(model, id); len(terms) > 0 {
			out = append(out, fitClaim{name: id.Name, volume: volume,
				terms: terms})
		}
	}
	return out
}

// claimTerms returns the node terms of a claim and, when they come from
// its bound volume, the volume's name.
func claimTerms(
	model inventory.Reader, claim inventory.EntityID,
) ([][]kube.Requirement, string) {
	if terms, id := referencedTerms(model, claim, kube.KindPV); terms != nil {
		return terms, id.Name
	}
	terms, _ := referencedTerms(model, claim, kube.KindStorageClass)
	return terms, ""
}

// referencedTerms reads the node terms of the first object of kind that
// claim references and that has any.
func referencedTerms(
	model inventory.Reader, claim inventory.EntityID, kind inventory.Kind,
) ([][]kube.Requirement, inventory.EntityID) {
	for _, id := range model.Related(claim, inventory.References,
		inventory.Outgoing) {
		if id.Kind != kind {
			continue
		}
		if e, ok := model.Entity(id); ok {
			if terms := kube.ParseNodeTerms(e); len(terms) > 0 {
				return terms, id
			}
		}
	}
	return nil, inventory.EntityID{}
}

// requirementText writes requirements as "k=v" where they name one
// value, else "k In [a b]".
func requirementText(reqs []kube.Requirement) string {
	parts := make([]string, 0, len(reqs))
	for _, r := range reqs {
		switch {
		case r.Op == "In" && len(r.Values) == 1:
			parts = append(parts, r.Key+"="+r.Values[0])
		case len(r.Values) == 0:
			parts = append(parts, r.Key+" "+r.Op)
		default:
			parts = append(parts, r.Key+" "+r.Op+" ["+
				strings.Join(r.Values, " ")+"]")
		}
	}
	return strings.Join(parts, ", ")
}
