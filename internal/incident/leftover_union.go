package incident

import (
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// linkClues splits clues into the groups their links join: the same
// anchor, the same API domain, or names that match in one namespace
// (leftover_link.go).
func linkClues(clues []leftoverClue) [][]leftoverClue {
	u := make(union, len(clues))
	for i := range u {
		u[i] = i
	}
	for _, in := range u.joinKeys(clues) {
		for a := range in {
			for _, b := range in[a+1:] {
				if namesLink(clues[in[a]], clues[b]) {
					u.join(in[a], b)
				}
			}
		}
	}
	groups := map[int][]leftoverClue{}
	for i, c := range clues {
		root := u.find(i)
		groups[root] = append(groups[root], c)
	}
	out := make([][]leftoverClue, 0, len(groups))
	for _, group := range groups {
		out = append(out, group)
	}
	return out
}

// joinKeys joins the clues that share an anchor or an API domain, and
// returns the clues with names by the namespace they are read in.
func (u union) joinKeys(clues []leftoverClue) map[string][]int {
	byAnchor := map[string]int{}
	byDomain := map[string]int{}
	byScope := map[string][]int{}
	for i, c := range clues {
		if !c.anchor.IsZero() {
			u.joinFirst(byAnchor, c.anchor.String(), i)
		}
		for _, domain := range c.domains {
			u.joinFirst(byDomain, domain, i)
		}
		if len(c.names) > 0 {
			byScope[c.scope] = append(byScope[c.scope], i)
		}
	}
	return byScope
}

// namesLink reports two clues of one namespace that name the same
// controller: a name in common, or a name of one that is named after
// an anchor's name of the other. Two clues without an anchor (two
// budgets, two certificates) never link by name: nothing says a
// controller is gone.
func namesLink(a, b leftoverClue) bool {
	if a.anchor.IsZero() && b.anchor.IsZero() {
		return false
	}
	for _, x := range a.names {
		for _, y := range b.names {
			if x == y || !a.anchor.IsZero() && kube.NamedAfter(y, x) ||
				!b.anchor.IsZero() && kube.NamedAfter(x, y) {
				return true
			}
		}
	}
	return false
}

// union is a disjoint-set forest over clue indexes.
type union []int

func (u union) find(i int) int {
	for u[i] != i {
		u[i] = u[u[i]]
		i = u[i]
	}
	return i
}

func (u union) join(a, b int) {
	if ra, rb := u.find(a), u.find(b); ra != rb {
		u[max(ra, rb)] = min(ra, rb)
	}
}

// joinFirst joins i with the first clue seen under key.
func (u union) joinFirst(first map[string]int, key string, i int) {
	if j, ok := first[key]; ok {
		u.join(j, i)
		return
	}
	first[key] = i
}
