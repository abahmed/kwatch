package detectors

import (
	"math"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// resourceAsk is one resource a pod asks for and how that resource is
// read from containers and nodes.
type resourceAsk struct {
	// blocker is the scheduler's wording when a node lacks it.
	blocker     string
	request     string
	allocatable string
	text        func(value float64, up bool) string
}

var (
	cpuAsk = resourceAsk{
		blocker: "insufficient cpu", request: kube.AttrCPUReq,
		allocatable: kube.AttrCPUAllocatable, text: cpuText,
	}
	memoryAsk = resourceAsk{
		blocker: "insufficient memory", request: kube.AttrMemoryReq,
		allocatable: kube.AttrMemoryAllocatable, text: memoryText,
	}
)

// capacityEvidence turns the scheduler's "Insufficient cpu/memory" into
// numbers: what the pod needs and the most any schedulable node has
// free. It says nothing when the pod declares no requests, since then
// the gap cannot be measured.
func capacityEvidence(
	model inventory.Reader, pod inventory.EntityID, message string,
) []detection.Evidence {
	blockers, _ := kube.ParseSchedulerMessage(message)
	var out []detection.Evidence
	for _, ask := range []resourceAsk{cpuAsk, memoryAsk} {
		if !blocked(blockers, ask.blocker) {
			continue
		}
		need := podRequest(model, pod, ask.request)
		if need <= 0 {
			continue
		}
		out = append(out, detection.Evidence{
			Label: "needs", Value: ask.text(need, true),
		})
		out = append(out, mostFree(model, ask))
	}
	return out
}

func blocked(blockers []kube.Blocker, reason string) bool {
	for _, b := range blockers {
		if strings.EqualFold(b.Reason, reason) {
			return true
		}
	}
	return false
}

// mostFree names the schedulable node with the most unrequested
// capacity of the resource.
func mostFree(
	model inventory.Reader, ask resourceAsk,
) detection.Evidence {
	best, bestNode, found := 0.0, "", false
	for _, id := range model.Entities(kube.KindNode) {
		node, ok := model.Entity(id)
		if !ok || !schedulableNode(node) {
			continue
		}
		total, ok := number(node, ask.allocatable)
		if !ok {
			continue
		}
		free := total - requestedOn(model, id, ask.request)
		if !found || free > best {
			best, bestNode, found = free, id.Name, true
		}
	}
	if !found {
		return detection.Evidence{Label: "most free",
			Value: "no schedulable node"}
	}
	return detection.Evidence{Label: "most free",
		Value: ask.text(math.Max(best, 0), false) + " on " + bestNode}
}

// schedulableNode is a node that is neither cordoned nor NotReady.
func schedulableNode(node inventory.Entity) bool {
	if flag(node, kube.AttrUnschedulable) {
		return false
	}
	if _, known := node.Attribute(kube.AttrReady); known {
		return flag(node, kube.AttrReady)
	}
	return true
}

// requestedOn sums the requests of the pods that still run on a node;
// finished pods have released theirs.
func requestedOn(
	model inventory.Reader, node inventory.EntityID, attr string,
) float64 {
	total := 0.0
	for _, id := range model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		if pod, ok := model.Entity(id); ok && podFinished(pod) {
			continue
		}
		total += podRequest(model, id, attr)
	}
	return total
}

// podRequest is what the scheduler reserves for a pod: the sum over its
// containers, or the largest init container when that is bigger.
func podRequest(
	model inventory.Reader, pod inventory.EntityID, attr string,
) float64 {
	sum, initMax := 0.0, 0.0
	for _, id := range model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		container, ok := model.Entity(id)
		if !ok {
			continue
		}
		value, _ := number(container, attr)
		if flag(container, kube.AttrInit) {
			initMax = math.Max(initMax, value)
		} else {
			sum += value
		}
	}
	return math.Max(sum, initMax)
}

// cpuText renders millicores: "250m CPU", "1.2 CPU". Rounding goes up
// for what a pod needs and down for what is free, so the gap never
// looks smaller than it is.
func cpuText(milli float64, up bool) string {
	if milli < 1000 {
		return strconv.Itoa(int(milli)) + "m CPU"
	}
	return decimal(milli/1000, up) + " CPU"
}

// memoryText renders bytes as MiB or GiB.
func memoryText(bytes float64, up bool) string {
	const mib, gib = 1 << 20, 1 << 30
	if bytes < gib {
		return strconv.Itoa(int(bytes/mib)) + " MiB memory"
	}
	return decimal(bytes/gib, up) + " GiB memory"
}

// decimal prints v with at most one decimal, dropping a trailing ".0".
func decimal(v float64, up bool) string {
	round := math.Floor
	if up {
		round = math.Ceil
	}
	// Trim float noise (1.2*10 is 12.000000000000002) before rounding.
	tenths := math.Round(v*10*1e6) / 1e6
	return strconv.FormatFloat(round(tenths)/10, 'f', -1, 64)
}
