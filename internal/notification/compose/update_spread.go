package compose

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// spreadSentences say what newly fails, one sentence per condition,
// led by the incident's subject: "payments in shop is spreading to
// payments-api and cart: they have not been ready for three minutes." A cause
// that now shows its own condition is not spreading: "API service m
// now reports …".
func spreadSentences(f caseFacts, spread []detection.Finding) []sentence {
	home := leadSubject(f)
	spread = slices.DeleteFunc(slices.Clone(spread),
		func(s detection.Finding) bool {
			return partOfRoot(f, s.Entity) || partOfPlace(f, s.Entity)
		})
	var order []string
	byState := map[string][]inventory.EntityID{}
	var out []sentence
	// named records that a sentence already led with the subject, so
	// the spreading sentence says "It" instead of naming it again.
	named := false
	for _, s := range spread {
		state := predicate(s.Entity, s.Summary)
		if s.Entity == f.p.Root || (f.p.Cause != nil &&
			s.Entity == f.p.Cause.Root) {
			out = append(out, sentence{part: partLead,
				text: endSentence(capitalName(s.Entity,
					f.leadName(s.Entity)+" "+nowState(state)))})
			named = named || f.leadName(s.Entity) == f.leadName(home)
			continue
		}
		if s.Entity == home {
			// The lead's own subject is not spreading to itself.
			continue
		}
		if _, seen := byState[state]; !seen {
			order = append(order, state)
		}
		byState[state] = append(byState[state], s.Entity)
	}
	return append(out, spreadGroupSentences(f, home, named,
		spreadGroups(f, order, byState))...)
}

// spreadGroup is a set of workloads and the states they are in.
type spreadGroup struct {
	ids    []inventory.EntityID
	states []string
}

// spreadGroups collects what newly fails by the workloads it touches: a
// workload that is both without replicas and Pending is one group with
// two states, not two sentences that name it twice.
func spreadGroups(
	f caseFacts, order []string, byState map[string][]inventory.EntityID,
) []spreadGroup {
	var groups []spreadGroup
	for _, state := range order {
		ids := workloadsOf(f, byState[state])
		merged := false
		for i := range groups {
			if slices.Equal(groups[i].ids, ids) {
				groups[i].states = append(groups[i].states, state)
				merged = true
			}
		}
		if !merged {
			groups = append(groups, spreadGroup{ids: ids,
				states: []string{state}})
		}
	}
	return groups
}

// spreadGroupSentences say what each group of workloads is spreading
// to, led by the incident's subject: "payments in shop is spreading to
// cart and search: they have not been ready for three minutes."
func spreadGroupSentences(
	f caseFacts, home inventory.EntityID, named bool, groups []spreadGroup,
) []sentence {
	homeName, many := f.subjectName(home)
	var out []sentence
	for i, g := range groups {
		if run, ok := lastRunSentence(home, homeName, named || i > 0,
			g); ok {
			out = append(out, sentence{part: partLead, text: run})
			continue
		}
		text := capitalName(home, homeName+" "+beWord(many)+
			" spreading to ")
		switch {
		case i > 0:
			text = pronoun(many) + " " + beWord(many) + " also " +
				"spreading to "
		case named:
			text = pronoun(many) + " " + beWord(many) + " spreading to "
		}
		text += nameList(home, g.ids) + ": "
		states := make([]string, 0, len(g.states))
		for _, state := range g.states {
			if len(g.ids) > 1 {
				state = pluralPredicate(state)
			}
			states = append(states, state)
		}
		subject := "it "
		if len(g.ids) > 1 {
			subject = "they "
		}
		out = append(out, sentence{part: partLead,
			text: endSentence(text + subject +
				strings.Join(states, " and "))})
	}
	return out
}

// partOfPlace reports a node of the zone or pool that fails as a whole:
// the place already says it, so the node is not news of its own.
func partOfPlace(f caseFacts, id inventory.EntityID) bool {
	return leadIsGroup(f) && id.Kind == kube.KindNode
}

// beWord is "is" for one subject and "are" for several.
func beWord(many bool) string {
	if many {
		return "are"
	}
	return "is"
}

// pronoun is "It" for one subject and "They" for several.
func pronoun(many bool) string {
	if many {
		return "They"
	}
	return "It"
}

// partOfRoot reports a pod or container that belongs to the incident's
// own root: a container of the root pod, or a pod of the root workload.
// The root failing through its parts is the root failing, not spreading.
func partOfRoot(f caseFacts, id inventory.EntityID) bool {
	root := f.p.Root
	switch id.Kind {
	case kube.KindContainer:
		pod, _ := splitContainer(id.Name)
		if root.Kind == kube.KindPod && root.Namespace == id.Namespace &&
			root.Name == pod {
			return true
		}
		id = inventory.CoreID(kube.KindPod, id.Namespace, pod)
		fallthrough
	case kube.KindPod:
		owner, ok := ownerIn(append([]inventory.EntityID{root},
			f.p.Impact...), id)
		return ok && owner == root
	}
	return false
}
