package detectors

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeKillContext states, for a container the node killed, the node's
// memory and what the container used against its request: the kernel
// and the kubelet pick first the pods that use most above their
// requests.
func nodeKillContext(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	out := []detection.Evidence{{Label: detection.EvidenceOOMKind,
		Value: "the node's memory"}}
	if node, ok := nodeOf(ctx, e); ok {
		if text := nodeMemoryText(ctx, node); text != "" {
			out = append(out, detection.Evidence{
				Label: detection.EvidenceNodeMemory, Value: text})
		}
	}
	used, known := usedBeforeKill(e)
	request, _ := number(e, kube.AttrMemoryReq)
	if text := againstRequest(used, request, known); text != "" {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceOverRequest, Value: text})
	}
	return out
}

// nodeMemoryText is "requested 11.2Gi of 12Gi allocatable; 11.8Gi in
// use", from the requests of the pods on the node and the working sets
// the kubelet last reported. A part kwatch does not know is left out.
func nodeMemoryText(ctx detection.Context, node inventory.Entity) string {
	t := nodeMemoryTotals(ctx, node.ID)
	var parts []string
	if alloc, ok := number(node, kube.AttrMemoryAllocatable); ok &&
		alloc > 0 && t.asked {
		parts = append(parts, "requested "+quantity(t.requested)+" of "+
			quantity(alloc)+" allocatable")
	}
	if t.seen {
		parts = append(parts, quantity(t.used)+" in use")
	}
	return strings.Join(parts, "; ")
}

// memoryTotals are the memory requests and use of a node's pods; asked
// and seen say whether any container had a request or a reading.
type memoryTotals struct {
	requested, used float64
	asked, seen     bool
}

func nodeMemoryTotals(
	ctx detection.Context, node inventory.EntityID,
) memoryTotals {
	var t memoryTotals
	for _, pod := range ctx.Model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		if podFinishedID(ctx.Model, pod) {
			continue
		}
		for _, c := range runningContainers(ctx, pod) {
			if r, ok := number(c, kube.AttrMemoryReq); ok && r > 0 {
				t.requested, t.asked = t.requested+r, true
			}
			if u, ok := number(c, kube.AttrMemoryWorking); ok {
				t.used, t.seen = t.used+u, true
			}
		}
	}
	return t
}

// againstRequest words a use against its request.
func againstRequest(used, request float64, known bool) string {
	switch {
	case !known:
		return ""
	case request <= 0:
		return "used " + quantity(used) + " with no memory request"
	case used > request:
		return "used " + quantity(used) + ", over its " +
			quantity(request) + " request"
	}
	return "used " + quantity(used) + ", within its " + quantity(request) +
		" request"
}

// limitBounds are the bounds a raised memory limit of e must fit: the
// namespace's LimitRange maximum and ResourceQuota room, the largest
// node, and the replicas that each get the raise. A kind kwatch cannot
// read is named instead, so the step says it was not checked.
func limitBounds(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	var out []detection.Evidence
	add := func(label, value string) {
		out = append(out, detection.Evidence{Label: label, Value: value})
	}
	var unread []string
	for _, b := range []struct {
		kind  inventory.Kind
		attr  string
		label string
		name  string
	}{
		{kube.KindLimitRange, kube.AttrLimitMaxMemory,
			detection.EvidenceMemoryMaxAllowed, "LimitRange"},
		{kube.KindQuota, kube.AttrQuotaMemoryLeft,
			detection.EvidenceMemoryQuotaLeft, "ResourceQuota"},
	} {
		if !ctx.Readable(b.kind) {
			unread = append(unread, b.name)
			continue
		}
		if least, ok := leastIn(ctx.Model, b.kind, e.ID.Namespace,
			b.attr); ok {
			add(b.label, quantity(least))
		}
	}
	if len(unread) > 0 {
		add(detection.EvidenceBoundsUnread, strings.Join(unread, ", "))
	}
	if largest := largestNodeMemory(ctx.Model); largest > 0 {
		add(detection.EvidenceMemoryNodeMax, quantity(largest))
	}
	if replicas := workloadReplicas(ctx, e); replicas > 1 {
		add(detection.EvidenceReplicas, strconv.Itoa(replicas))
	}
	return out
}

// leastIn is the smallest attr of the objects of kind in namespace.
func leastIn(
	model inventory.Reader, kind inventory.Kind, namespace, attr string,
) (float64, bool) {
	least, found := 0.0, false
	for _, id := range model.Entities(kind) {
		if id.Namespace != namespace {
			continue
		}
		entity, ok := model.Entity(id)
		if v, has := number(entity, attr); ok && has &&
			(!found || v < least) {
			least, found = v, true
		}
	}
	return least, found
}

// largestNodeMemory is the largest allocatable memory of a node.
func largestNodeMemory(model inventory.Reader) float64 {
	largest := 0.0
	for _, id := range model.Entities(kube.KindNode) {
		if node, ok := model.Entity(id); ok {
			v, _ := number(node, kube.AttrMemoryAllocatable)
			largest = max(largest, v)
		}
	}
	return largest
}

// workloadReplicas is how many replicas the container's workload runs,
// or 0 when it has no workload with a replica count.
func workloadReplicas(ctx detection.Context, e inventory.Entity) int {
	pod, ok := owningPod(ctx, e)
	if !ok {
		return 0
	}
	owner, ok := ctx.Model.Entity(inventory.TopOwner(ctx.Model, pod.ID))
	if !ok || owner.ID == pod.ID {
		return 0
	}
	replicas, _ := number(owner, kube.AttrReplicas)
	return int(replicas)
}

// evictionPressureSlack is how long after the eviction the node's memory
// pressure may have ended and still be the pressure that evicted it.
const evictionPressureSlack = 10 * time.Minute

// evictionMemoryContext states, for a pod the kubelet evicted while its
// node was under memory pressure, the node's memory, its biggest users
// and what the pod used against its requests. It says nothing for an
// eviction under any other pressure.
func evictionMemoryContext(
	ctx detection.Context, pod inventory.Entity, evictedAt time.Time,
) []detection.Evidence {
	if ctx.Model == nil {
		return nil
	}
	nodes := ctx.Model.Related(pod.ID, inventory.RunsOn, inventory.Outgoing)
	if len(nodes) == 0 {
		return nil
	}
	node, ok := ctx.Model.Entity(nodes[0])
	status, _, since := condition(node, "MemoryPressure")
	if !ok || !(status == "True" || status == "False" &&
		!evictedAt.IsZero() && since.After(evictedAt) &&
		since.Sub(evictedAt) <= evictionPressureSlack) {
		return nil
	}
	out := []detection.Evidence{{Label: detection.EvidenceOOMKind,
		Value: "the node's memory"}}
	if text := nodeMemoryText(ctx, node); text != "" {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceNodeMemory, Value: text})
	}
	used, request, known := podPeakAndRequest(ctx, pod.ID)
	if text := againstRequest(used, request, known); text != "" {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceOverRequest, Value: text})
	}
	for _, user := range biggestUsers(ctx, node.ID, pod.ID) {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceNodeMemoryUser, Value: user})
	}
	return out
}

// podPeakAndRequest sums the 24 hour memory peaks and the requests of a
// pod's containers. known is false when no container has a peak.
func podPeakAndRequest(
	ctx detection.Context, pod inventory.EntityID,
) (used, request float64, known bool) {
	for _, c := range runningContainers(ctx, pod) {
		if peak, ok := number(c, kube.AttrMemoryPeak24h); ok && peak > 0 {
			used, known = used+peak, true
		}
		if r, ok := number(c, kube.AttrMemoryReq); ok {
			request += r
		}
	}
	return used, request, known
}
