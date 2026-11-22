package explain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkExhaustsMemory links a node that ran out of memory to the
// workload whose process the kernel named as the victim.
const LinkExhaustsMemory LinkType = "exhausts-memory"

// ModeMemoryHog is a workload that runs nodes out of memory one after
// the other: the kernel names its process in a system OOM event on a
// node, and its replacement does the same on the next. It is a pseudo
// mode read from the nodes' events.
const ModeMemoryHog detection.Mode = "MemoryHog"

// memoryHogRow blames a workload whose process was the system OOM
// victim on more than one node for the trouble of each of them.
var memoryHogRows = []Row{{
	Name: "workload-exhausts-memory",
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeMemoryHog}},
	Link: LinkExhaustsMemory,
	Effect: Side{Kind: kube.KindNode, Modes: []detection.Mode{
		detection.ModeMemoryPressure, detection.ModeHeartbeatStale,
		detection.ModeKubeletUnreachable, detection.ModeNotReady}},
	Prior: 0.8,
}, {
	// The pods of the nodes it took down are its effect too.
	Name: "workload-exhausts-memory",
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeMemoryHog}},
	Link: LinkExhaustsMemory,
	Effect: podSide(
		detection.ModeNotReady, detection.ModeStatusUnknown,
		detection.ModeEvicted, detection.ModeUnreachable,
		detection.ModeFailed, detection.ModeStuckDeleting,
		detection.ModeProbe, detection.ModePending),
	Prior: 0.8,
}}

var oomVictim = regexp.MustCompile(`victim process: ([^,\s]+)`)

// exhaustsMemory reports a candidate chosen as the workload that runs
// nodes out of memory; like a disk hog, what fails behind it are the
// nodes, not workloads that depend on it.
func exhaustsMemory(c *candidate) bool {
	return c.bestMatch().match.row.Name == "workload-exhausts-memory"
}

// memoryHogOutcome is the shared evidence of a hog: the kernel named
// its process on each of the nodes among the effects.
func memoryHogOutcome(effects []inventory.EntityID) outcome {
	nodes := map[inventory.EntityID]bool{}
	for _, effect := range effects {
		if effect.Kind == kube.KindNode {
			nodes[effect] = true
		}
	}
	return outcome{weight: SharedWeight,
		code: rootcause.ProofExhaustsMemory, count: len(nodes),
		text: fmt.Sprintf("the kernel killed its process for memory "+
			"on %d nodes", len(nodes))}
}

// oomVictims are the processes the kernel named in the node's system
// OOM events.
func (v *view) oomVictims(node inventory.EntityID) map[string]bool {
	out := map[string]bool{}
	for _, n := range v.s.Model.Notes(node, v.since) {
		if n.Reason != "SystemOOM" {
			continue
		}
		if m := oomVictim.FindStringSubmatch(n.Message); m != nil {
			out[m[1]] = true
		}
	}
	return out
}

// memoryHogHops lead from a node with a system OOM to the workload
// whose process the kernel killed, when another node lost the same
// process too: the fault moved with the workload. One node with one
// victim stays the node's incident.
func (v *view) memoryHogHops(node inventory.EntityID) []hop {
	for _, name := range sortedStrings(v.oomVictims(node)) {
		hog, ok := v.workloadRunning(name)
		if ok && !v.failsItself(hog) && v.oomedElsewhere(node, name) {
			return []hop{{link: LinkExhaustsMemory, to: hog}}
		}
	}
	return nil
}

// failsItself reports a workload with a finding of its own on it, its
// pods or containers: the kernel killed it as a victim, and what
// explains its failure is its own.
func (v *view) failsItself(top inventory.EntityID) bool {
	for id, found := range v.s.Findings {
		if len(found) > 0 && id.Kind != kube.KindNode &&
			rootcause.TopOwner(v.s.Model, id) == top {
			return true
		}
	}
	return false
}

func (v *view) oomedElsewhere(node inventory.EntityID, name string) bool {
	for _, other := range v.s.Model.Entities(kube.KindNode) {
		if other == node {
			continue
		}
		v.gate(node, other)
		if v.oomVictims(other)[name] {
			return true
		}
	}
	return false
}

// workloadRunning is the single workload that runs the process: it has
// a container named or imaged for it, or, once its pods are gone, is
// named for it. Two workloads sharing the name make it unknown.
func (v *view) workloadRunning(
	process string,
) (top inventory.EntityID, ok bool) {
	for _, c := range v.s.Model.Entities(kube.KindContainer) {
		e, found := v.s.Model.Entity(c)
		if !found || !runsProcess(c, e, process) {
			continue
		}
		pods := v.s.Model.Related(c, inventory.PartOf, inventory.Outgoing)
		if len(pods) == 0 {
			continue
		}
		owner := rootcause.TopOwner(v.s.Model, pods[0])
		if ok && top != owner {
			return inventory.EntityID{}, false
		}
		top, ok = owner, true
	}
	if ok {
		return top, true
	}
	for _, kind := range []inventory.Kind{kube.KindDeployment,
		kube.KindStatefulSet, kube.KindDaemonSet} {
		for _, id := range v.s.Model.Entities(kind) {
			if id.Name != process {
				continue
			}
			if ok {
				return inventory.EntityID{}, false
			}
			top, ok = id, true
		}
	}
	return top, ok
}

// runsProcess reports a container that plausibly runs the process: it
// is named for it or its image is.
func runsProcess(
	id inventory.EntityID, e inventory.Entity, process string,
) bool {
	if _, name, ok := strings.Cut(id.Name, "/"); ok && name == process {
		return true
	}
	image := attrText(e, kube.AttrImage)
	image, _, _ = strings.Cut(image, "@")
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		image = image[:i]
	}
	return image[strings.LastIndex(image, "/")+1:] == process
}

// memoryHogModes is ModeMemoryHog for a workload seen from a node
// whose memory its process exhausted, or from a pod on that node.
func (v *view) memoryHogModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkExhaustsMemory {
		return nil
	}
	nodes := []inventory.EntityID{effect}
	if effect.Kind != kube.KindNode {
		nodes = v.s.Model.Related(effect, inventory.RunsOn,
			inventory.Outgoing)
	}
	for _, node := range nodes {
		for _, h := range v.memoryHogHops(node) {
			if h.to == id {
				return pseudoFailing(ModeMemoryHog)
			}
		}
	}
	return nil
}

func sortedStrings(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
