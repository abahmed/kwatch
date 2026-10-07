package detectors

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// maxFitGroups bounds the pools named in one explanation.
const maxFitGroups = 3

// nodeVerdict is one node and what stops the pod from using it.
type nodeVerdict struct {
	node     fitNode
	blockers []blocker
}

// verdict checks every node and writes the explanation. It says nothing
// when some node passes every check, and nothing when the numbers the
// scheduler's message already carries are the whole story: one pool
// short of a resource or held by a taint.
func (c *fitCase) verdict() []detection.Evidence {
	if c.spec.TooBig {
		return fitNote("the pod's scheduling rules are too large for " +
			"kwatch to check")
	}
	if len(c.nodes) == 0 {
		return nil
	}
	results := make([]nodeVerdict, len(c.nodes))
	for i, n := range c.nodes {
		results[i] = nodeVerdict{n, c.check(n)}
	}
	if len(passingNodes(results)) > 0 {
		// Some node passes every check kwatch can make: what stops the
		// pod is something it cannot see, and it has nothing to add.
		return nil
	}
	global := sharedBlockers(results)
	groups := bestPerGroup(results, global)
	if redundantFit(results, global, groups) {
		return nil
	}
	var out []detection.Evidence
	for _, b := range global {
		out = append(out, detection.Evidence{Label: detection.EvidenceFit,
			Value: globalText(b)})
	}
	volumes, served := c.volumeNodes(results)
	out = append(out, volumes...)
	groups = withoutNodes(groups, served)
	out = append(out, groupEvidence(groups)...)
	out = append(out, wouldFitEvidence(groups)...)
	return append(out, c.limitNotes(results)...)
}

func fitNote(value string) []detection.Evidence {
	return []detection.Evidence{{Label: detection.EvidenceFitNote,
		Value: value}}
}

func passingNodes(results []nodeVerdict) []string {
	var names []string
	for _, r := range results {
		if len(r.blockers) == 0 {
			names = append(names, r.node.id.Name)
		}
	}
	return names
}

// sharedBlockers are the blockers every node has, such as a volume that
// can attach where no node is. They are reported once, not per pool.
func sharedBlockers(results []nodeVerdict) []blocker {
	counts := map[string]int{}
	first := map[string]blocker{}
	var order []string
	for _, r := range results {
		for _, b := range r.blockers {
			if b.kind != blockLabel && b.kind != blockVolume {
				continue
			}
			if counts[b.text] == 0 {
				order = append(order, b.text)
				first[b.text] = b
			}
			counts[b.text]++
		}
	}
	var out []blocker
	for _, text := range order {
		if counts[text] == len(results) {
			out = append(out, first[text])
		}
	}
	return out
}

func globalText(b blocker) string {
	if b.all != "" {
		return b.all
	}
	return "every node " + b.text
}

// groupBest is the node of a pool nearest to taking the pod.
type groupBest struct {
	group   string
	best    nodeVerdict
	blocked []blocker
}

// bestPerGroup picks, for each pool, the node with the fewest blockers
// left once the shared ones are set aside, and orders the pools nearest
// first. A pool whose best node has no other blocker has room.
func bestPerGroup(results []nodeVerdict, shared []blocker) []groupBest {
	skip := map[string]bool{}
	for _, b := range shared {
		skip[b.text] = true
	}
	best := map[string]groupBest{}
	for _, r := range results {
		var left []blocker
		for _, b := range r.blockers {
			if !skip[b.text] {
				left = append(left, b)
			}
		}
		if len(left) == 0 {
			continue
		}
		cur, seen := best[r.node.group]
		if !seen || nearer(left, cur.blocked) {
			best[r.node.group] = groupBest{r.node.group, r, left}
		}
	}
	out := make([]groupBest, 0, len(best))
	for _, g := range best {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if nearer(out[i].blocked, out[j].blocked) !=
			nearer(out[j].blocked, out[i].blocked) {
			return nearer(out[i].blocked, out[j].blocked)
		}
		return out[i].group < out[j].group
	})
	return out
}

// nearer reports whether blockers a leave the pod closer to fitting
// than b: lighter blockers first, then the smaller resource shortfall.
func nearer(a, b []blocker) bool {
	if wa, wb := weight(a), weight(b); wa != wb {
		return wa < wb
	}
	return shortfall(a) < shortfall(b)
}

// volumeWeight makes a node outside the volume's zone farther than any
// number of other blockers can: the volume cannot follow the pod.
const volumeWeight = 100

func weight(list []blocker) int {
	total := 0
	for _, b := range list {
		if b.kind == blockVolume {
			total += volumeWeight
		} else {
			total++
		}
	}
	return total
}

func shortfall(list []blocker) float64 {
	total := 0.0
	for _, b := range list {
		if b.kind == blockResource && b.need > 0 {
			total += (b.need - b.free) / b.need
		}
	}
	return total
}

// redundantFit is true for one pool where every node is blocked only by
// resources or taints: the scheduler's message and the numbers already
// say it.
func redundantFit(
	results []nodeVerdict, shared []blocker, groups []groupBest,
) bool {
	if len(shared) > 0 || len(groups) != 1 {
		return false
	}
	for _, r := range results {
		for _, b := range r.blockers {
			if b.kind != blockResource && b.kind != blockTaint {
				return false
			}
		}
	}
	return true
}

func groupEvidence(groups []groupBest) []detection.Evidence {
	var out []detection.Evidence
	for i, g := range groups {
		if i == maxFitGroups {
			out = append(out, detection.Evidence{
				Label: detection.EvidenceFit,
				Value: "and " + strconv.Itoa(len(groups)-i) +
					" more groups of nodes"})
			break
		}
		out = append(out, detection.Evidence{Label: detection.EvidenceFit,
			Value: groupText(g)})
	}
	return out
}

func groupLabel(group string) string {
	if group == "" {
		return "the nodes"
	}
	return "pool " + group
}

// groupText writes what stops the pod on the best node of a pool.
func groupText(g groupBest) string {
	label, node := groupLabel(g.group), g.best.node.id.Name
	switch {
	case allKind(g.blocked, blockResource):
		return label + " has no node with " + resourceList(g.blocked, true) +
			" free (best: " + node + " has " +
			resourceList(g.blocked, false) + " free)"
	case allKind(g.blocked, blockTaint):
		return label + " has room but " + taintWords(g.blocked)
	}
	texts := make([]string, 0, len(g.blocked))
	for _, b := range g.blocked {
		texts = append(texts, b.text)
	}
	return label + ": the closest node, " + node + ", " +
		strings.Join(texts, " and ")
}

func allKind(list []blocker, kind blockKind) bool {
	for _, b := range list {
		if b.kind != kind {
			return false
		}
	}
	return len(list) > 0
}

// resourceList writes the short resources: what the pod needs of each
// (up) or what the best node has of each.
func resourceList(list []blocker, needed bool) string {
	parts := make([]string, 0, len(list))
	for _, b := range list {
		if needed {
			parts = append(parts, resourceText(b.resource, b.need, true))
		} else {
			parts = append(parts, resourceText(b.resource, b.free, false))
		}
	}
	return strings.Join(parts, " and ")
}

func taintWords(list []blocker) string {
	names := make([]string, 0, len(list))
	for _, b := range list {
		names = append(names, b.taint)
	}
	if len(names) == 1 {
		return "taint " + names[0] + " isn't tolerated"
	}
	return "taints " + strings.Join(names, " and ") + " aren't tolerated"
}

// wouldFitEvidence states, for pools whose best node has only lifted-able
// blockers, what change would make the pod fit there. It is a fact about
// the cluster, not an instruction.
func wouldFitEvidence(groups []groupBest) []detection.Evidence {
	var out []detection.Evidence
	for _, g := range groups {
		lifts := make([]string, 0, len(g.blocked))
		for _, b := range g.blocked {
			if b.lift == "" {
				lifts = nil
				break
			}
			lifts = append(lifts, b.lift)
		}
		if len(lifts) == 0 {
			continue
		}
		out = append(out, detection.Evidence{
			Label: detection.EvidenceFitWould,
			Value: strings.Join(lifts, " and ") + " would fit it on " +
				g.best.node.id.Name})
		if len(out) == 2 {
			break
		}
	}
	return out
}

// limitNotes say what the check had to leave out.
func (c *fitCase) limitNotes(results []nodeVerdict) []detection.Evidence {
	var out []detection.Evidence
	if c.total > len(c.nodes) {
		out = append(out, fitNote("checked "+strconv.Itoa(len(c.nodes))+
			" of "+strconv.Itoa(c.total)+" nodes")...)
	}
	unread := 0
	for _, r := range results {
		if !r.node.labelsOK {
			unread++
		}
	}
	if unread > 0 {
		out = append(out, fitNote(strconv.Itoa(unread)+
			" nodes have too many labels for kwatch to read")...)
	}
	return out
}
