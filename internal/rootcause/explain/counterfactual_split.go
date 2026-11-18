package explain

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Replica split bounds: the fewest failing and healthy pods a split
// needs, and the most differences it reports.
const (
	SplitMinFailing     = DifferenceMinFailing
	SplitMinHealthy     = 1
	SplitMaxDifferences = 2
)

// Split attributes, in the order ReplicaSplit tries them.
const (
	SplitDigest   = "digest"
	SplitRevision = "revision"
	SplitArch     = "architecture"
	SplitInstance = "instance type"
	SplitZone     = "zone"
	SplitNode     = "node"
	SplitQoS      = "qos"
)

// revisionLabel is the label a StatefulSet or DaemonSet puts on the
// pods of one revision.
const revisionLabel = "controller-revision-hash"

// SplitDifference is one attribute that cleanly separates failing
// replicas from healthy ones: every failing pod has Failing and no
// healthy pod has it. Healthy is the value every healthy pod shares,
// "" when they differ among themselves. Image is the tag whose digest
// differs, for SplitDigest; Owner says "ReplicaSet" when Failing names
// a ReplicaSet, for SplitRevision.
type SplitDifference struct {
	Attribute string
	Failing   string
	Healthy   string
	Image     string `json:",omitempty"`
	Owner     string `json:",omitempty"`
}

// ReplicaSplit compares the failing replicas of one workload with its
// healthy ones, the way the counterfactual difference check does, and
// returns at most SplitMaxDifferences attributes that cleanly separate
// them, strongest first. It returns nothing with fewer than
// SplitMinFailing failing or SplitMinHealthy healthy pods, or when no
// attribute separates them: a mixed split says nothing.
func ReplicaSplit(
	model inventory.Reader, failing, healthy []inventory.EntityID,
) []SplitDifference {
	if model == nil || len(failing) < SplitMinFailing ||
		len(healthy) < SplitMinHealthy {
		return nil
	}
	v := &view{s: Snapshot{Model: model}}
	bad, good := v.traitsList(failing), v.traitsList(healthy)
	var out []SplitDifference
	if d, ok := digestSplit(bad, good); ok {
		out = append(out, d)
	}
	for _, a := range splitAttributes {
		if len(out) == SplitMaxDifferences {
			break
		}
		if x, y, ok := cleanSplit(bad, good, a.get); ok {
			out = append(out, a.difference(x, y))
		}
	}
	return out[:min(len(out), SplitMaxDifferences)]
}

// splitAttribute reads one attribute of a pod's traits.
type splitAttribute struct {
	name string
	get  func(podTraits) string
}

// difference words a clean split of a: a revision that is not a hash
// (marked "#") names a ReplicaSet.
func (a splitAttribute) difference(failing, healthy string) SplitDifference {
	out := SplitDifference{Attribute: a.name, Failing: failing,
		Healthy: healthy}
	if a.name != SplitRevision {
		return out
	}
	if !strings.HasPrefix(failing, "#") {
		out.Owner = "ReplicaSet"
	}
	out.Failing = strings.TrimPrefix(failing, "#")
	out.Healthy = strings.TrimPrefix(healthy, "#")
	return out
}

// splitAttributes are tried after the digest, in this order.
var splitAttributes = []splitAttribute{
	{SplitRevision, func(t podTraits) string { return t.revision }},
	{SplitArch, func(t podTraits) string { return t.arch }},
	{SplitInstance, func(t podTraits) string { return t.instance }},
	{SplitZone, func(t podTraits) string { return t.zone }},
	{SplitNode, func(t podTraits) string { return t.node }},
	{SplitQoS, func(t podTraits) string { return t.qos }},
}

// cleanSplit is the value every failing pod shares and no healthy pod
// has, with the value the healthy pods share ("" when they differ).
func cleanSplit(
	bad, good []podTraits, get func(podTraits) string,
) (failing, healthy string, ok bool) {
	failing = uniform(bad, get)
	if failing == "" {
		return "", "", false
	}
	for _, t := range good {
		if value := get(t); value == "" || value == failing {
			return "", "", false
		}
	}
	return failing, uniform(good, get), true
}

// digestSplit is a tag every pod runs whose digest cleanly separates
// the failing pods from the healthy ones.
func digestSplit(bad, good []podTraits) (SplitDifference, bool) {
	refs := make([]string, 0, len(bad[0].builds))
	for ref := range bad[0].builds {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		get := func(t podTraits) string { return t.builds[ref] }
		if x, y, ok := cleanSplit(bad, good, get); ok {
			return SplitDifference{Attribute: SplitDigest, Image: ref,
				Failing: x, Healthy: y}, true
		}
	}
	return SplitDifference{}, false
}

// placeTraits records where a pod runs and what it was made from: its
// node with the node's architecture and instance type, the ReplicaSet
// that owns it (else its controller revision hash, marked with "#")
// and its QoS class.
func (v *view) placeTraits(pod inventory.EntityID, t *podTraits) {
	if e, ok := v.s.Model.Entity(pod); ok {
		t.qos = attrText(e, kube.AttrQoS)
		labels := kube.ParseLabels(attrText(e, kube.AttrLabels))
		if hash := labels[revisionLabel]; hash != "" {
			t.revision = "#" + hash
		}
	}
	if owner, ok := v.directOwner(pod); ok &&
		owner.Kind == kube.KindReplicaSet {
		t.revision = owner.Name
	}
	node, ok := v.nodeOf(pod)
	if !ok {
		return
	}
	t.node = node.Name
	if e, ok := v.s.Model.Entity(node); ok {
		labels := kube.ParseLabels(attrText(e, kube.AttrNodeLabels))
		t.arch = strings.TrimSpace(labels["kubernetes.io/arch"])
		t.instance = attrText(e, kube.AttrInstanceType)
	}
}
