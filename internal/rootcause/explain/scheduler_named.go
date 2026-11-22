package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// ModeNamedByScheduler is an object the scheduler's verdict on a pod
// points at: its words are about a setting, a claim or a node that one
// object owns, and that object explains the Pending better than the
// reason alone. The scheduler's reasons are read against the pod; each
// entry of schedulerNamers says what object a kind of reason points at.
const ModeNamedByScheduler detection.Mode = "NamedByScheduler"

// schedulerNamer reads one kind of scheduling reason. match says whether
// the verdict is of its kind; named returns the objects it points at. Its
// mode and rule are its own, so that each kind reads in its own words.
type schedulerNamer struct {
	mode  detection.Mode
	rule  string
	match func(verdict string) bool
	named func(v *view, pod inventory.EntityID, verdict string,
	) []inventory.EntityID
}

// schedulerNamers are the reasons that point at an object, in order; the
// first that names something decides.
var schedulerNamers = []schedulerNamer{
	{ // The pod's PriorityClass never preempts.
		mode: ModeNamedByScheduler + ".PriorityClass",
		rule: "scheduler-names-priority-class",
		match: func(s string) bool {
			return strings.Contains(s, "preemptionpolicy=never")
		},
		named: func(v *view, pod inventory.EntityID, _ string,
		) []inventory.EntityID {
			return v.referenced(pod, kube.KindPriorityClass)
		}},
	{ // The pod conflicts with its own workload's old pod.
		mode: ModeNamedByScheduler + ".Owner",
		rule: "scheduler-names-owner",
		match: func(s string) bool {
			return strings.Contains(s, "readwriteoncepod")
		},
		named: func(v *view, pod inventory.EntityID, _ string,
		) []inventory.EntityID {
			return []inventory.EntityID{rootcause.TopOwner(v.s.Model, pod)}
		}},
	{ // No node has room for the claim the pod mounts.
		mode: ModeNamedByScheduler + ".Claim",
		rule: "scheduler-names-claim",
		match: func(s string) bool {
			return strings.Contains(s, "enough free storage")
		},
		named: func(v *view, pod inventory.EntityID, _ string,
		) []inventory.EntityID {
			return v.claimsOf(pod)
		}},
	{ // The pod asks to sit next to pods that no workload runs.
		mode: ModeNamedByScheduler + ".AffinityTarget",
		rule: "scheduler-names-affinity-target",
		match: func(s string) bool {
			return strings.Contains(s, "pod affinity rules")
		},
		named: (*view).affinityTargets},
	{ // The pod's topology spread key is a label no node has.
		mode: ModeNamedByScheduler + ".Spread",
		rule: "scheduler-names-spread",
		match: func(s string) bool {
			// Without the label the constraint's key names, no node
			// can count: the spread setting, not capacity, is at fault.
			return strings.Contains(s, "pod topology spread "+
				"constraints (missing required label)")
		},
		named: (*view).spreadOwner},
	{ // A node is cordoned and the rest have no room: the cordon is what
		// left the pod without a place.
		mode: ModeNamedByScheduler + ".CordonedNode",
		rule: "scheduler-names-cordoned-node",
		match: func(s string) bool {
			return strings.Contains(s, "were unschedulable") &&
				strings.Contains(s, "insufficient ")
		},
		named: (*view).cordonedNodes},
	{ // The pod is pinned to one node by name and that node has no room.
		mode:  ModeNamedByScheduler + ".PinnedNode",
		rule:  "scheduler-names-pinned-node",
		match: func(s string) bool { return strings.Contains(s, "insufficient") },
		named: (*view).pinnedNode},
	{ // A node lists the scarce resource at zero.
		mode: ModeNamedByScheduler + ".ScarceNode",
		rule: "scheduler-names-scarce-node",
		match: func(s string) bool {
			return strings.Contains(s, "insufficient ")
		},
		named: (*view).nodesLackingResource},
}

// schedulerMessage is the scheduler's full verdict on the pod, lower
// case.
func (v *view) schedulerMessage(pod inventory.EntityID) string {
	for _, f := range v.s.Findings[pod] {
		for _, e := range f.Evidence {
			if e.Label == "scheduler" {
				return strings.ToLower(e.Value)
			}
		}
	}
	return ""
}

// schedulerNamed is the object the scheduler's verdict on pod points at,
// with the kind of reason that named it.
func (v *view) schedulerNamed(
	pod inventory.EntityID,
) (schedulerNamer, []inventory.EntityID) {
	verdict := v.schedulerMessage(pod)
	if verdict == "" {
		return schedulerNamer{}, nil
	}
	for _, namer := range schedulerNamers {
		if !namer.match(verdict) {
			continue
		}
		if out := namer.named(v, pod, verdict); len(out) > 0 {
			return namer, out
		}
	}
	return schedulerNamer{}, nil
}

// namedModes is ModeNamedByScheduler for an object the effect pod's
// verdict names.
func (v *view) namedModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkSchedules && link != LinkMounts && link != LinkUses {
		return nil
	}
	pod, ok := v.unitOf(effect)
	if !ok {
		return nil
	}
	namer, objects := v.schedulerNamed(pod)
	for _, named := range objects {
		if named == id {
			return pseudoFailing(namer.mode)
		}
	}
	return nil
}

// referenced lists what the pod references of one kind.
func (v *view) referenced(
	pod inventory.EntityID, kind inventory.Kind,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range v.s.Model.Related(
		pod, inventory.References, inventory.Outgoing) {
		if id.Kind == kind {
			out = append(out, id)
		}
	}
	return out
}

// affinityTargets are the workloads whose pods the pod's required pod
// affinity selects and that run no ready pod: scaled to zero or down.
func (v *view) affinityTargets(
	pod inventory.EntityID, _ string,
) []inventory.EntityID {
	entity, ok := v.s.Model.Entity(pod)
	if !ok {
		return nil
	}
	spec := kube.ParseSchedulingSpec(entity)
	var out []inventory.EntityID
	for _, term := range spec.Affinity {
		out = append(out, v.unreadyTargets(pod.Namespace, term.Selector)...)
	}
	return out
}

// unreadyTargets are the workloads of the namespace that the selector
// matches and that have no ready replica.
func (v *view) unreadyTargets(
	namespace string, selector []kube.Requirement,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, kind := range workloadKinds {
		for _, id := range v.s.Model.EntitiesIn(kind, namespace) {
			w, found := v.s.Model.Entity(id)
			if !found || !v.selectsLabels(selector, w) {
				continue
			}
			if ready, _ := attrNumber(w, kube.AttrReadyReplicas); ready == 0 {
				out = append(out, id)
			}
		}
	}
	return out
}

// selectsLabels reports whether the selector matches the labels of the
// pods the workload creates.
func (v *view) selectsLabels(
	selector []kube.Requirement, workload inventory.Entity,
) bool {
	if len(selector) == 0 {
		return false
	}
	labels := kube.ParseLabels(attrText(workload, kube.AttrTemplateLabels))
	for _, r := range selector {
		if !r.Matches(labels) {
			return false
		}
	}
	return true
}

// pinnedNode is the one node a DaemonSet-style pod is pinned to by name,
// when the verdict says a node was short of a resource.
func (v *view) pinnedNode(
	pod inventory.EntityID, _ string,
) []inventory.EntityID {
	if node, found := v.nodeOf(pod); found {
		return []inventory.EntityID{node}
	}
	entity, ok := v.s.Model.Entity(pod)
	if !ok {
		return nil
	}
	for _, term := range kube.ParseSchedulingSpec(entity).NodeTerms {
		for _, r := range term {
			if r.Key == kube.FieldPrefix+"metadata.name" &&
				len(r.Values) == 1 {
				return []inventory.EntityID{inventory.CoreID(
					kube.KindNode, "", r.Values[0])}
			}
		}
	}
	return nil
}

// nodesLackingResource are the nodes that list the scarce extended
// resource the verdict names at zero allocatable.
func (v *view) nodesLackingResource(
	_ inventory.EntityID, verdict string,
) []inventory.EntityID {
	_, after, found := strings.Cut(verdict, "insufficient ")
	if !found {
		return nil
	}
	resource := strings.TrimRight(strings.Fields(after)[0], ".,:")
	var out []inventory.EntityID
	for _, id := range v.s.Model.Entities(kube.KindNode) {
		node, ok := v.s.Model.Entity(id)
		if !ok {
			continue
		}
		for _, item := range strings.Split(
			attrText(node, kube.AttrAllocatable), ",") {
			if strings.EqualFold(item, resource+"=0") {
				out = append(out, id)
			}
		}
	}
	return out
}

// schedulerNamesRows say that the object a pod's scheduling verdict
// points at explains the Pending. A walk keeps one link per target, and
// the object may be reached by the pod's own relation to it (it mounts
// the claim, it uses the class), so each kind of reason has a row for
// each link.
func schedulerNamesRows() []Row {
	var rows []Row
	for _, namer := range schedulerNamers {
		for _, link := range []LinkType{
			LinkSchedules, LinkMounts, LinkUses} {
			rows = append(rows, Row{
				Name: namer.rule,
				Cause: Side{Group: AnyGroup, Kind: AnyKind,
					Modes: []detection.Mode{namer.mode}},
				Link: link,
				Effect: podSide(detection.ModePending,
					detection.ModeUnschedulable),
				Prior: 0.8,
			})
		}
	}
	return rows
}

// cordonedNodes are the nodes marked unschedulable.
func (v *view) cordonedNodes(
	_ inventory.EntityID, _ string,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range v.s.Model.Entities(kube.KindNode) {
		if node, ok := v.s.Model.Entity(id); ok &&
			attrBool(node, kube.AttrUnschedulable) {
			out = append(out, id)
		}
	}
	return out
}

// spreadOwner is the workload whose pod sets the topology spread
// constraint the scheduler names: the constraint is in the pod's spec,
// so the workload that owns the spec is what to read.
func (v *view) spreadOwner(
	pod inventory.EntityID, _ string,
) []inventory.EntityID {
	entity, ok := v.s.Model.Entity(pod)
	if !ok || len(kube.ParseSchedulingSpec(entity).Spread) == 0 {
		return nil
	}
	return []inventory.EntityID{rootcause.TopOwner(v.s.Model, pod)}
}
