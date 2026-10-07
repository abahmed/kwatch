package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// The counterfactual checks ask "what if it were this cause?" and look
// for something healthy that the cause would have broken too: the same
// image in another workload, the same ConfigMap in another workload,
// the other replicas, the other pods on the node, the previous
// revision. A healthy twin rules a cause out; its absence rules it in.
// Each check is a scorer (counterfactualScorers) and records what it
// compared in Trace.Compared, so a message can say what was checked.

// counterfactualScorers are added to scorers; see scorer.go.
var counterfactualScorers = []scorer{
	{"image-elsewhere", scoreImageElsewhere},
	{"config-elsewhere", scoreConfigElsewhere},
	{"replicas", scoreReplicas},
	{"difference", scoreDifference},
	{"node-peers", scoreNodePeers},
	{"previous-revision", scorePreviousHealthy},
}

// blame says what kind of thing a candidate blames.
type blame int

const (
	// blamesOther is anything the checks do not judge.
	blamesOther blame = iota
	// blamesImage is an image, or a release that changed the image.
	blamesImage
	// blamesSurroundings is config, environment or a dependency: what
	// surrounds the image.
	blamesSurroundings
	// blamesPlace is a node, zone or node pool.
	blamesPlace
)

// blamesOf classifies a candidate.
func (v *view) blamesOf(c *candidate) blame {
	switch c.id.Kind {
	case kube.KindImage:
		return blamesImage
	case kube.KindNode, kube.KindZone, kube.KindNodePool:
		return blamesPlace
	case kube.KindConfigMap, kube.KindSecret, kube.KindAccount,
		kube.KindService, KindExternalEndpoint:
		return blamesSurroundings
	}
	if c.bestMatch().match.row.Name == "image-drift" {
		return blamesImage
	}
	return v.blamesChange(c.id)
}

// blamesChange classifies a workload by what its recent changes
// touched: the image, or something else in the spec.
func (v *view) blamesChange(id inventory.EntityID) blame {
	out := blamesOther
	for _, change := range v.changesOf(id) {
		for _, field := range change.Fields {
			if strings.HasSuffix(field.Path, ".image") {
				return blamesImage
			}
			out = blamesSurroundings
		}
	}
	return out
}

// failingUnit is one failure the candidate explains with its failing
// pods.
type failingUnit struct {
	effect inventory.EntityID
	pods   []inventory.EntityID
}

// failingUnits lists a sample of the failures a candidate explains,
// each with its failing pods.
func (v *view) failingUnits(c *candidate) []failingUnit {
	effects := sampleIDs(c.direct())
	if len(effects) > ExclusivitySample {
		effects = effects[:ExclusivitySample]
	}
	var out []failingUnit
	for _, effect := range effects {
		var pods []inventory.EntityID
		if pod, ok := v.podOf(effect); ok {
			pods = []inventory.EntityID{pod}
		} else if slices.Contains(kube.WorkloadKinds, effect.Kind) {
			pods = sampleIDs(v.ownedPods(effect))
		}
		var failing []inventory.EntityID
		for _, pod := range pods {
			if v.unitFailing(pod) {
				failing = append(failing, pod)
			}
		}
		if len(failing) > 0 {
			out = append(out, failingUnit{effect: effect, pods: failing})
		}
	}
	return out
}

// workloadsOfUnits is the set of workloads the failing pods belong to.
func (v *view) workloadsOfUnits(
	units []failingUnit,
) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for _, u := range units {
		for _, pod := range u.pods {
			out[v.workloadOf(pod)] = true
		}
	}
	return out
}

// healthyPod reports whether a pod is up, ready and shows no failure
// on itself or its containers.
func (v *view) healthyPod(pod inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(pod)
	if !ok {
		return false
	}
	ready, _ := attributeValue(e, kube.AttrReady).AsBool()
	deleting, _ := attributeValue(e, kube.AttrDeleting).AsBool()
	return ready && !deleting && !v.unitFailing(pod)
}

// healthyWorkload reports whether pod is healthy and no pod of its
// workload fails: a workload half down is not a healthy twin.
func (v *view) healthyWorkload(pod inventory.EntityID) bool {
	if !v.healthyPod(pod) {
		return false
	}
	return !v.anyFailing(v.ownedPods(v.workloadOf(pod)))
}

// ownedPods lists the pods a controller owns.
func (v *view) ownedPods(id inventory.EntityID) []inventory.EntityID {
	return rootcause.OwnedPods(v.s.Model, id)
}

// compare records one comparison for a failure, once.
func (v *view) compare(effect inventory.EntityID, cmp Comparison) {
	if v.compared == nil {
		v.compared = map[inventory.EntityID][]Comparison{}
	}
	for _, known := range v.compared[effect] {
		if known.Code == cmp.Code && known.With == cmp.With {
			return
		}
	}
	v.compared[effect] = append(v.compared[effect], cmp)
}

// shortName is "namespace/name", or the name of a cluster object.
func shortName(id inventory.EntityID) string {
	if id.Namespace == "" {
		return id.Name
	}
	return id.Namespace + "/" + id.Name
}

// comparedInArea is the compared map restricted to the area's failures;
// nil when none of them was compared.
func (v *view) comparedInArea(
	inArea map[inventory.EntityID]bool,
) map[inventory.EntityID][]Comparison {
	var out map[inventory.EntityID][]Comparison
	for effect, list := range v.compared {
		if !inArea[effect] {
			continue
		}
		if out == nil {
			out = map[inventory.EntityID][]Comparison{}
		}
		out[effect] = list
	}
	return out
}
