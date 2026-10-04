package explain

import "github.com/abahmed/kwatch/internal/inventory"

// specificEffects are the failures a viable candidate with evidence of
// its own explains: neither a workload blamed for itself nor a shared
// factor. Any cause with evidence outranks a shared factor, so the
// shared factor must not take these failures over.
func specificEffects(viable []scored) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for _, s := range viable {
		if s.c.isSelf() || sharedFactorOnly(s.c) {
			continue
		}
		for effect := range s.c.covers {
			out[effect] = true
		}
	}
	return out
}

// sharedGain counts the uncovered failures a shared-factor-only
// candidate may claim: those no specific cause explains. It claims
// none when fewer than SharedFactorMinWorkloads remain, because the
// rest no longer look like failures that began together.
func sharedGain(
	s scored, uncovered, specific map[inventory.EntityID]bool,
) int {
	n := 0
	for effect := range s.c.covers {
		if uncovered[effect] && !specific[effect] {
			n++
		}
	}
	if n < SharedFactorMinWorkloads {
		return 0
	}
	return n
}

// coverGain is how many uncovered failures s may claim.
func coverGain(
	s scored, uncovered, specific map[inventory.EntityID]bool,
) int {
	if sharedFactorOnly(s.c) {
		return sharedGain(s, uncovered, specific)
	}
	return countCovered(s, uncovered)
}

// claimCovered removes the failures a chosen candidate explains from
// uncovered. A shared factor leaves the specific causes' failures.
func claimCovered(
	s scored, uncovered, specific map[inventory.EntityID]bool,
) {
	shared := sharedFactorOnly(s.c)
	for effect := range s.c.covers {
		if !shared || !specific[effect] {
			delete(uncovered, effect)
		}
	}
}
