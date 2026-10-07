package detectors

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// blockKind says which constraint stops a pod from using a node.
type blockKind int

const (
	blockState blockKind = iota
	blockTaint
	blockLabel
	blockVolume
	blockResource
	blockPlacement
)

// blocker is one reason a node cannot take the pod.
type blocker struct {
	kind blockKind
	// text finishes "the node ...": "is cordoned".
	text string
	// all words the blocker when every node has it: "no node has ...".
	// Empty means "every node " + text.
	all string
	// lift is what would remove the blocker, as a clause that finishes
	// "... would fit it on n1": "tolerating gpu=true:NoSchedule".
	lift string
	// taint is the taint a toleration would lift.
	taint string
	// resource, need and free describe a resource shortfall.
	resource   string
	need, free float64
}

// check lists every constraint that stops the pod from using the node.
func (c *fitCase) check(n fitNode) []blocker {
	out := stateBlockers(n)
	out = append(out, taintBlockers(c.spec, n)...)
	out = append(out, labelBlockers(c.spec, n)...)
	out = append(out, c.volumeBlockers(n)...)
	out = append(out, c.resourceBlockers(n)...)
	out = append(out, c.placementBlockers(n)...)
	return out
}

func stateBlockers(n fitNode) []blocker {
	var out []blocker
	if n.cordoned {
		out = append(out, blocker{kind: blockState, text: "is cordoned"})
	}
	if n.notReady {
		out = append(out, blocker{kind: blockState, text: "is NotReady"})
	}
	return out
}

// stateTaints are the taints that only repeat a state reported as
// such.
var stateTaints = map[string]bool{
	"node.kubernetes.io/unschedulable": true,
	"node.kubernetes.io/not-ready":     true,
	"node.kubernetes.io/unreachable":   true,
}

func taintBlockers(spec kube.SchedulingSpec, n fitNode) []blocker {
	var out []blocker
	for _, taint := range n.taints {
		if taint.Effect == "PreferNoSchedule" ||
			(stateTaints[taint.Key] && (n.cordoned || n.notReady)) {
			continue
		}
		if !tolerated(spec.Tolerations, taint) {
			out = append(out, blocker{kind: blockTaint,
				taint: taint.String(),
				lift:  "tolerating " + taint.String(),
				text: "has taint " + taint.String() + ", which the pod " +
					"doesn't tolerate"})
		}
	}
	return out
}

func tolerated(list []kube.Toleration, taint kube.Taint) bool {
	for _, t := range list {
		if t.Tolerates(taint) {
			return true
		}
	}
	return false
}

// labelBlockers checks the node selector and the required node
// affinity. A node whose labels were too many to keep is not judged.
func labelBlockers(spec kube.SchedulingSpec, n fitNode) []blocker {
	if !n.labelsOK {
		return nil
	}
	var out []blocker
	keys := make([]string, 0, len(spec.Selector))
	for key := range spec.Selector {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		want := spec.Selector[key]
		if have, ok := n.labels[key]; !ok || have != want {
			out = append(out, blocker{kind: blockLabel,
				text: "lacks the label " + key + "=" + want +
					" the pod selects",
				all: "no node has the label " + key + "=" + want +
					" the pod selects",
				lift: "dropping the selector " + key + "=" + want})
		}
	}
	if len(spec.NodeTerms) > 0 && !anyTermMatches(spec.NodeTerms, n.labels) {
		affinity := requirementText(spec.NodeTerms[0])
		out = append(out, blocker{kind: blockLabel,
			text: "does not meet the pod's node affinity (" + affinity + ")",
			all:  "no node meets the pod's node affinity (" + affinity + ")",
			lift: "dropping its node affinity"})
	}
	return out
}

func anyTermMatches(
	terms [][]kube.Requirement, labels map[string]string,
) bool {
	for _, term := range terms {
		if kube.MatchesAll(term, labels) {
			return true
		}
	}
	return false
}

// volumeBlockers checks where the pod's volume claims can attach.
func (c *fitCase) volumeBlockers(n fitNode) []blocker {
	if !n.labelsOK {
		return nil
	}
	var out []blocker
	for _, claim := range c.claims {
		if !anyTermMatches(claim.terms, n.labels) {
			out = append(out, blocker{kind: blockVolume,
				text: "is not where claim " + claim.name +
					" can attach (" + requirementText(claim.terms[0]) + ")",
				all: claim.nowhere()})
		}
	}
	return out
}

// resourceBlockers compares what the pod needs with what is left.
func (c *fitCase) resourceBlockers(n fitNode) []blocker {
	var out []blocker
	for _, key := range sortedResources(c.need) {
		free, known := n.free[key]
		if !known || free >= c.need[key] {
			continue
		}
		have := max(free, 0)
		out = append(out, blocker{kind: blockResource, resource: key,
			need: c.need[key], free: have,
			text: "has only " + resourceText(key, have, false) +
				" free of the " + resourceText(key, c.need[key], true) +
				" it needs"})
	}
	return out
}

func sortedResources(need map[string]float64) []string {
	keys := make([]string, 0, len(need))
	for key := range need {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return resourceRank(keys[i]) < resourceRank(keys[j]) ||
			resourceRank(keys[i]) == resourceRank(keys[j]) &&
				keys[i] < keys[j]
	})
	return keys
}

func resourceRank(key string) int {
	switch key {
	case resCPU:
		return 0
	case resMemory:
		return 1
	case resEphemeral:
		return 2
	case resPods:
		return 4
	}
	return 3
}

// resourceText writes an amount of a resource in words.
func resourceText(key string, value float64, up bool) string {
	switch key {
	case resCPU:
		return cpuText(value, up)
	case resMemory:
		return memoryText(value, up)
	case resEphemeral:
		return strings.Replace(memoryText(value, up), " memory",
			" ephemeral storage", 1)
	case resPods:
		if value == 1 {
			return "1 pod slot"
		}
		return strconv.Itoa(int(value)) + " pod slots"
	}
	return decimal(value, up) + " " + key
}
