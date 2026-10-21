package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkFillsDisk links a node showing disk pressure to the workload whose
// pods fill its disk.
const LinkFillsDisk LinkType = "fills-disk"

// ModeDiskHog is a workload that fills the disks of the nodes it runs
// on, one after the other: the kubelet evicts its pod from one node and
// the replacement fills the next. It is a pseudo mode read from the
// nodes' own pods.
const ModeDiskHog detection.Mode = "DiskHog"

// diskNodeModes are the findings of a node whose disk is short.
var diskNodeModes = []detection.Mode{
	detection.ModeDiskPressure, detection.ModeDisk,
	detection.ModeFilesystem, detection.ModeInodes}

// diskHogRow blames a workload that fills the disk of more than one
// node for the pressure on each of them.
var diskHogRow = Row{
	Name: "workload-fills-disks",
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeDiskHog}},
	Link:   LinkFillsDisk,
	Effect: Side{Kind: kube.KindNode, Modes: diskNodeModes},
	Prior:  0.8,
}

// fillsDisks reports a candidate chosen as the workload that fills the
// disks of the nodes it runs on. What fails behind it are pods evicted
// from those nodes, not workloads that depend on it, so the "workloads
// meet here" evidence does not apply to it.
func fillsDisks(c *candidate) bool {
	return c.bestMatch().match.row.Name == diskHogRow.Name
}

// diskHogOutcome is the shared evidence of a hog: its pods hold most of
// the disk in use on each of the nodes among the effects it is blamed
// for, which is what those nodes have in common. It weighs what
// "workloads meet here" would.
func diskHogOutcome(effects []inventory.EntityID) outcome {
	nodes := map[inventory.EntityID]bool{}
	for _, effect := range effects {
		if effect.Kind == kube.KindNode {
			nodes[effect] = true
		}
	}
	return outcome{weight: SharedWeight, code: rootcause.ProofFillsDisks,
		count: len(nodes), text: fmt.Sprintf(
			"its pods hold most of the disk in use on %d nodes",
			len(nodes))}
}

// diskHogHops lead from a node with disk pressure to the workload that
// fills its disk, when that workload fills another node's disk too. One
// node with one hog stays the node's incident: the fault moved with the
// workload only when it showed up on a second node.
func (v *view) diskHogHops(node inventory.EntityID) []hop {
	hog, ok := v.hogOf(node, true)
	if !ok || !v.hogsAnotherNode(hog, node) {
		return nil
	}
	return []hop{{link: LinkFillsDisk, to: hog}}
}

// hogOf is the workload that fills the node's disk: the kubelet evicted
// one of its pods from the node within ExposureWindow, and its pods hold
// at least SharedFactorMinShare of the ephemeral storage the node's pods
// use. Evicted pods keep the last usage the kubelet reported. The node
// need show pressure now only when pressure is asked for: the other
// node of a workload that moved may have recovered.
func (v *view) hogOf(
	node inventory.EntityID, pressure bool,
) (inventory.EntityID, bool) {
	if pressure && !v.hasMode(node, diskNodeModes) {
		return inventory.EntityID{}, false
	}
	used := map[inventory.EntityID]float64{}
	evicted := map[inventory.EntityID]bool{}
	total := 0.0
	for _, pod := range v.s.Model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		v.gate(node, pod)
		top := rootcause.TopOwner(v.s.Model, pod)
		if v.recentlyEvicted(pod) {
			evicted[top] = true
		}
		if e, ok := v.s.Model.Entity(pod); ok {
			bytes, _ := attrNumber(e, kube.AttrEphemeralUsed)
			used[top] += bytes
			total += bytes
		}
	}
	for _, top := range sortedKeys(evicted) {
		if total > 0 && used[top] >= total*SharedFactorMinShare {
			return top, true
		}
	}
	return inventory.EntityID{}, false
}

// recentlyEvicted reports a pod the kubelet evicted within
// ExposureWindow of now, read from the pod's own record: the eviction
// finding itself ends sooner than the pod stays in the API.
func (v *view) recentlyEvicted(pod inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(pod)
	if !ok || attrText(e, kube.AttrReason) != "Evicted" {
		return false
	}
	phase, ok := e.Attribute(kube.AttrPhase)
	return ok && phase.Since.After(v.s.Now.Add(-ExposureWindow))
}

// hogsAnotherNode reports another node whose disk the same workload
// filled: the kubelet evicted its pod there for it.
func (v *view) hogsAnotherNode(
	hog, node inventory.EntityID,
) bool {
	for _, other := range v.s.Model.Entities(kube.KindNode) {
		if other == node {
			continue
		}
		v.gate(node, other)
		if found, ok := v.hogOf(other, false); ok && found == hog {
			return true
		}
	}
	return false
}

// hogModes is ModeDiskHog for a workload seen from the node whose disk
// it fills.
func (v *view) hogModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkFillsDisk || effect.Kind != kube.KindNode {
		return nil
	}
	if hog, ok := v.hogOf(effect, true); ok && hog == id &&
		v.hogsAnotherNode(hog, effect) {
		return pseudoFailing(ModeDiskHog)
	}
	return nil
}
