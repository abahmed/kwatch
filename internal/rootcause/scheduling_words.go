package rootcause

import "strings"

// SchedulingWords words the blocker a virtual scheduling entity is
// named after, as a clause a reader understands: "no node has enough
// memory", "no node matches its node affinity". A blocker without
// plainer words is quoted after "the scheduler rejects every node".
func SchedulingWords(reason string) string {
	reason = SchedulingReason(reason)
	lower := strings.ToLower(reason)
	switch {
	case strings.HasPrefix(reason, "Insufficient "):
		return "no node has enough " +
			strings.TrimPrefix(reason, "Insufficient ")
	case strings.Contains(lower, "node affinity/selector"):
		return "no node matches its node affinity or node selector"
	case strings.Contains(lower, "nodeaffinity"),
		strings.Contains(lower, "node affinity"):
		return "no node matches its node affinity"
	}
	return "the scheduler rejects every node (" + reason + ")"
}

// SchedulingReason is a scheduler blocker without what newer schedulers
// write after it in the same item: "didn't satisfy plugin(s)
// [NodeAffinity]. no new claims to deallocate," is "didn't satisfy
// plugin(s) [NodeAffinity]". Dots inside a word (nvidia.com/gpu) stay.
func SchedulingReason(reason string) string {
	if before, _, found := strings.Cut(reason, ". "); found {
		reason = before
	}
	return strings.TrimSpace(strings.TrimRight(reason, ",. "))
}
