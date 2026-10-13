package explain

import "github.com/abahmed/kwatch/internal/inventory"

// The common-factor helpers answer one question for every cause that
// rests on "what the failing members share that the healthy ones lack":
// the zone, the digest or the containers that separate failing replicas
// from healthy ones, and the share of a shared node's pods that fail.
// What a factor is worth stays with each caller; these only count.

// uniform returns the one value every pod has, or "" when the pods
// disagree or one has none.
func uniform(pods []podTraits, get func(podTraits) string) string {
	out := ""
	for i, p := range pods {
		value := get(p)
		if value == "" || (i > 0 && value != out) {
			return ""
		}
		out = value
	}
	return out
}

// separator is the factor that cleanly splits failing pods from healthy
// ones: every failing pod has the value failing, every healthy pod has
// the value healthy, and the two differ. ok is false when either side
// disagrees with itself or the two sides agree.
func separator(
	bad, good []podTraits, get func(podTraits) string,
) (failing, healthy string, ok bool) {
	failing, healthy = uniform(bad, get), uniform(good, get)
	if failing == "" || healthy == "" || failing == healthy {
		return "", "", false
	}
	return failing, healthy, true
}

// shareOf counts how many of the members satisfy has, and how many
// there are.
func shareOf(
	members []inventory.EntityID, has func(inventory.EntityID) bool,
) (with, total int) {
	for _, member := range members {
		if has(member) {
			with++
		}
	}
	return with, len(members)
}

// anyMember reports whether some member satisfies has.
func anyMember(
	members []inventory.EntityID, has func(inventory.EntityID) bool,
) bool {
	with, _ := shareOf(members, has)
	return with > 0
}
