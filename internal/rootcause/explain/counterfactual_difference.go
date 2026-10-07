package explain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreDifference compares failing replicas with healthy ones of the
// same owner and looks for one attribute that cleanly separates them:
// every failing pod has value X and no healthy pod has it. The
// attributes are the digest a mutable tag resolved to, a container only
// some pods carry (an injected sidecar, another sidecar image) and the
// zone. Replicas of one owner share a spec, so what separates them is
// not in the workload's own definition: a cause that blames the
// workload loses weight. A separating digest favours a cause that
// blames the image instead. A node is left to scoreReplicas and a
// place cause to exclusivity. One failing pod or no healthy pod says
// nothing: there is nothing to separate.
func scoreDifference(v *view, c *candidate) outcome {
	kind := v.blamesOf(c)
	if kind == blamesPlace {
		return outcome{}
	}
	for _, g := range v.replicaGroups(c) {
		d, ok := v.separation(g)
		if !ok {
			continue
		}
		v.compareGroup(g, rootcause.ProofReplicasDiffer, d.text)
		weight := -DifferenceWeight
		if d.digest && kind == blamesImage {
			weight = DifferenceWeight
		}
		return outcome{weight: weight, code: rootcause.ProofReplicasDiffer,
			count: len(g.failing), total: len(g.failing) + len(g.healthy),
			text: d.text}
	}
	return outcome{}
}

// difference is one separating attribute, in words.
type difference struct {
	text   string
	digest bool
}

// separation finds the first clean separator of a group's failing and
// healthy pods: digest, then containers, then zone.
func (v *view) separation(g replicaGroup) (difference, bool) {
	if len(g.failing) < DifferenceMinFailing || len(g.healthy) == 0 {
		return difference{}, false
	}
	bad, good := v.traitsList(g.failing), v.traitsList(g.healthy)
	head := fmt.Sprintf("%d of %d pods fail; ", len(g.failing),
		len(g.failing)+len(g.healthy))
	if text, ok := digestDifference(bad, good); ok {
		return difference{text: text, digest: true}, true
	}
	if text, ok := containerDifference(head, bad, good); ok {
		return difference{text: text}, true
	}
	badZone := uniform(bad, func(t podTraits) string { return t.zone })
	goodZone := uniform(good, func(t podTraits) string { return t.zone })
	if badZone != "" && goodZone != "" && badZone != goodZone {
		return difference{text: fmt.Sprintf("%sall run in zone %s, the %d "+
			"healthy ones in zone %s", head, badZone, len(good),
			goodZone)}, true
	}
	return difference{}, false
}

func (v *view) traitsList(pods []inventory.EntityID) []podTraits {
	out := make([]podTraits, 0, len(pods))
	for _, pod := range pods {
		out = append(out, v.traitsOf(pod))
	}
	return out
}

// digestDifference finds an image reference, a tag, that every pod
// runs but whose digest differs between the failing and the healthy
// pods, each side running one digest of its own.
func digestDifference(bad, good []podTraits) (string, bool) {
	refs := make([]string, 0, len(bad[0].builds))
	for ref := range bad[0].builds {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		get := func(t podTraits) string { return t.builds[ref] }
		x, y := uniform(bad, get), uniform(good, get)
		if x == "" || y == "" || x == y {
			continue
		}
		text := fmt.Sprintf("same tag %s, but the failing pods run %s",
			ref, x)
		if seen := bad[0].seen[ref]; !seen.IsZero() {
			text += " (seen " + seen.UTC().Format("15:04") + ")"
		}
		return text + ", the healthy ones " + y, true
	}
	return "", false
}

// containerDifference reports containers that all failing pods carry
// and no healthy pod does.
func containerDifference(
	head string, bad, good []podTraits,
) (string, bool) {
	set := func(t podTraits) string { return t.containerSet() }
	x, y := uniform(bad, set), uniform(good, set)
	if x == "" || y == "" || x == y {
		return "", false
	}
	extra := bad[0].extraContainers(good[0])
	if len(extra) == 0 {
		return "", false
	}
	return head + "all run extra container " +
		strings.Join(extra, ", ") + ", the healthy ones do not", true
}
