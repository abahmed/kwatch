package incident

import (
	"slices"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Leftovers of one removed controller that no detector ties together.
// A removed mesh control plane leaves a budget that selects nothing, an
// autoscaler of its gone Deployment and an injector webhook whose
// Service has no pods: three findings of three detectors, about three
// objects, each low alone. A detector marks only what it sees (the
// webhook's backend, leftover_group.go in detectors); linkLeftovers
// reads every low finding of the snapshot and links those that name
// the same controller:
//
//   - the same anchor: the absent backend a detector marked, the target
//     an autoscaler lost, the release stuck uninstalling;
//   - the same name, in the anchor's namespace: a budget selecting
//     app=mesh, an autoscaler of Deployment mesh, a Service mesh; an
//     anchor or an object named after another anchor (kube.NamedAfter):
//     release "guard" and Service "guard-svc", certificate
//     "guard-svc.tls";
//   - the same API domain: objects stuck on finalizer
//     runs.example.com/cleanup and a webhook named
//     mutate.runs.example.com.
//
// Each linked group of at least two objects with an anchor is marked
// a leftover of its best anchor (rootRank): a workload, else a Service,
// else a release, else, for workloads parked at zero replicas that
// routes still reach, their namespace. Only findings low alone are
// read: a live failure is never a leftover and keeps its own incident.

// linkLeftovers marks the findings of s and transitions that are
// leftovers of one removed controller with it (detection.Finding's
// Leftover). The snapshot's findings are copied, never changed in
// place: the caller still owns them.
func linkLeftovers(
	s explain.Snapshot, transitions []detection.Transition,
) (explain.Snapshot, []detection.Transition) {
	if s.Model == nil {
		return s, transitions
	}
	marks := leftoverMarks(s)
	if len(marks) == 0 {
		return s, transitions
	}
	findings := make(map[inventory.EntityID][]detection.Finding,
		len(s.Findings))
	for id, list := range s.Findings {
		findings[id] = markAll(list, marks)
	}
	s.Findings = findings
	out := slices.Clone(transitions)
	for i := range out {
		out[i].Finding = marked(out[i].Finding, marks)
	}
	return s, out
}

// markAll is list with each marked finding marked; list itself when
// none is.
func markAll(
	list []detection.Finding, marks map[detection.Key]inventory.EntityID,
) []detection.Finding {
	var out []detection.Finding
	for i, f := range list {
		if root, ok := marks[f.Key()]; ok && f.Leftover != root {
			if out == nil {
				out = slices.Clone(list)
			}
			out[i].Leftover = root
		}
	}
	if out == nil {
		return list
	}
	return out
}

// marked is f with its mark, if it has one.
func marked(
	f detection.Finding, marks map[detection.Key]inventory.EntityID,
) detection.Finding {
	if root, ok := marks[f.Key()]; ok {
		f.Leftover = root
	}
	return f
}

// leftoverMarks maps each finding of a linked group to the group's
// root.
func leftoverMarks(
	s explain.Snapshot,
) map[detection.Key]inventory.EntityID {
	var clues []leftoverClue
	for _, list := range s.Findings {
		for _, f := range list {
			if c, ok := clueOf(s.Model, f); ok {
				clues = append(clues, c)
			}
		}
	}
	if len(clues) < minLeftovers {
		return nil
	}
	marks := map[detection.Key]inventory.EntityID{}
	for _, group := range linkClues(clues) {
		root, ok := groupRoot(group)
		if !ok || objects(group) < minLeftovers {
			continue
		}
		for _, c := range group {
			marks[c.key] = root
		}
	}
	return marks
}

// groupRoot is the best anchor of group: the highest rootRank, then the
// first by name. ok is false when no anchor may root it.
func groupRoot(group []leftoverClue) (inventory.EntityID, bool) {
	var best inventory.EntityID
	rank := 0
	for _, c := range group {
		r := rootRank(c.anchor)
		if r > rank || r == rank && r > 0 &&
			c.anchor.String() < best.String() {
			best, rank = c.anchor, r
		}
	}
	return best, rank > 0
}

// rootRank orders the anchors that may root a group of leftovers: the
// controller workload, its Service, its Helm release, a namespace of
// parked workloads. Zero for anything else.
func rootRank(id inventory.EntityID) int {
	switch {
	case slices.Contains(kube.WorkloadKinds, id.Kind):
		return 4
	case id.Kind == kube.KindService:
		return 3
	case id.Kind == kube.KindHelmRelease:
		return 2
	case id.Kind == kube.KindNamespace:
		return 1
	}
	return 0
}

// objects counts the distinct objects of a group.
func objects(group []leftoverClue) int {
	seen := map[inventory.EntityID]bool{}
	for _, c := range group {
		seen[c.entity] = true
	}
	return len(seen)
}
