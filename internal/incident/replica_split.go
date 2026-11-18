package incident

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// ReplicaSplit is what the failing pods of one workload share that its
// healthy pods do not, when only some of them fail. Lines say each
// difference as an observation ("Only the 3 pods on arm64 nodes fail;
// the 4 pods on amd64 nodes are healthy."); Differences are the same
// in data. It never says the difference is the cause.
type ReplicaSplit struct {
	Workload    inventory.EntityID
	Failing     int
	Healthy     int
	Differences []explain.SplitDifference
	Lines       []string
}

// ReplicaSplitOf compares the failing pods of p with the healthy pods
// of the same workload. nil unless every failing pod belongs to one
// workload, at least explain.SplitMinFailing fail and
// explain.SplitMinHealthy are healthy, and some attribute cleanly
// separates them (explain.ReplicaSplit).
func ReplicaSplitOf(model inventory.Reader, p Incident) *ReplicaSplit {
	if model == nil {
		return nil
	}
	workload, failing, ok := failingPodsOf(model, p)
	if !ok {
		return nil
	}
	healthy := healthyPodsOf(model, workload, failing)
	bad := sortedIDs(failing)
	diffs := explain.ReplicaSplit(model, bad, healthy)
	if len(diffs) == 0 {
		return nil
	}
	out := &ReplicaSplit{Workload: workload, Failing: len(bad),
		Healthy: len(healthy), Differences: diffs}
	for _, d := range diffs {
		out.Lines = append(out.Lines, splitLine(d, len(bad), len(healthy)))
	}
	return out
}

// failingPodsOf are p's failing pods, when they all belong to one
// workload that is not a pod itself.
func failingPodsOf(model inventory.Reader, p Incident) (
	inventory.EntityID, map[inventory.EntityID]bool, bool,
) {
	pods := map[inventory.EntityID]bool{}
	var workload inventory.EntityID
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		pod, ok := rootcause.PodOf(model, f.Entity)
		if !ok {
			continue
		}
		owner := rootcause.TopOwner(model, pod)
		if owner == pod || (!workload.IsZero() && owner != workload) {
			return inventory.EntityID{}, nil, false
		}
		workload, pods[pod] = owner, true
	}
	return workload, pods, len(pods) > 0
}

// healthyPodsOf are the workload's pods that are not failing, ready
// and not being deleted, sorted.
func healthyPodsOf(
	model inventory.Reader, workload inventory.EntityID,
	failing map[inventory.EntityID]bool,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, pod := range rootcause.OwnedPods(model, workload) {
		if failing[pod] {
			continue
		}
		e, ok := model.Entity(pod)
		if !ok {
			continue
		}
		ready, _ := e.Attribute(kube.AttrReady)
		deleting, _ := e.Attribute(kube.AttrDeleting)
		up, _ := ready.Value.AsBool()
		gone, _ := deleting.Value.AsBool()
		if up && !gone {
			out = append(out, pod)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

// sortedIDs is the set's members, sorted.
func sortedIDs(set map[inventory.EntityID]bool) []inventory.EntityID {
	out := make([]inventory.EntityID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

// splitPlaces word each attribute: the phrase after "pods" for a value
// the failing pods share, and the one for healthy pods that differ.
var splitPlaces = map[string][2]string{
	explain.SplitArch:     {"on %s nodes", "on other nodes"},
	explain.SplitInstance: {"on %s nodes", "on other node types"},
	explain.SplitZone:     {"in zone %s", "in other zones"},
	explain.SplitNode:     {"on node %s", "on other nodes"},
	explain.SplitQoS:      {"of QoS class %s", "of other QoS classes"},
	explain.SplitRevision: {"of revision %s", "of other revisions"},
	explain.SplitDigest:   {"running %s", "running other builds"},
}

// splitLine says one difference: "Only the 3 pods on arm64 nodes
// fail; the 4 pods on amd64 nodes are healthy."
func splitLine(d explain.SplitDifference, failing, healthy int) string {
	words := splitPlaces[d.Attribute]
	bad := placeWords(words[0], d, d.Failing)
	good := words[1]
	if d.Healthy != "" {
		good = placeWords(words[0], d, d.Healthy)
	}
	verb := " are healthy."
	if healthy == 1 {
		verb = " is healthy."
	}
	return "Only the " + strconv.Itoa(failing) + " pods " + bad +
		" fail; the " + podCount(healthy) + " " + good + verb
}

// placeWords fills one phrase of splitPlaces with value.
func placeWords(phrase string, d explain.SplitDifference,
	value string,
) string {
	switch {
	case d.Attribute == explain.SplitRevision && d.Owner != "":
		phrase = "of " + d.Owner + " %s"
	case d.Attribute == explain.SplitDigest:
		value = d.Image + " as " + value
	}
	return strings.Replace(phrase, "%s", value, 1)
}

// podCount is "1 pod" or "4 pods".
func podCount(n int) string {
	if n == 1 {
		return "1 pod"
	}
	return strconv.Itoa(n) + " pods"
}
