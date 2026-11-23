package compose

// Wording and category of the rows that blame a node with no room for a
// pod, a headless Service that hides peers, a release no node can take
// and a cordoned node.
func init() {
	for rule, w := range map[string]causeWording{
		"node-refuses-pods": {
			words: "has no room for the pod, by its kubelet's count",
		},
		"headless-hides-not-ready-peers": {
			words: "answers only for ready pods, which wait for each other's names",
		},
		"rollout-unschedulable": {
			words: "rolled out a change shortly before, and no node can take its pods",
		},
		"scheduler-names-spread": {
			words: "sets a topology spread constraint that no node satisfies",
		},
		"scheduler-names-cordoned-node": {
			words: "is cordoned, which leaves the waiting pod without a place",
		},
	} {
		causeWords[rule] = w
	}
	for rule, c := range map[string]string{
		"node-refuses-pods":              categoryNode,
		"headless-hides-not-ready-peers": categoryDependency,
		"rollout-unschedulable":          categoryRollout,
		"scheduler-names-spread":         categoryScheduling,
		"scheduler-names-cordoned-node":  categoryScheduling,
	} {
		causeCategories[rule] = c
	}
}
