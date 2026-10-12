package scenarios

import "time"

// oomVerdictScenarios pair a kill at the container's own limit with a
// kill far below it, on a node that is short on memory in both: the
// node is blamed only for the kill it caused.
func oomVerdictScenarios() []scenario {
	return []scenario{oomLimitOnPressuredNode(),
		oomUnderLimitOnPressuredNode()}
}

// oomLimitOnPressuredNode: ledger used 489Mi of its 512Mi limit; the
// node reports memory pressure two minutes later, with a hog holding
// most of its memory. The kill is the container's own limit, so the
// node is its own incident and not the cause of the kill.
func oomLimitOnPressuredNode() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-limit-on-pressured-node",
			Description: "A container is OOM-killed at 95% of its limit " +
				"on a node under memory pressure; the limit is the " +
				"cause, not the node.",
			Root: "deployment/billing/ledger", Tier: "notify",
			OtherRoots:  []string{"node//n3"},
			MaxMessages: 5,
			Tail:        duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			k := pressuredKill(489)
			k.pressure, k.pressureAfter = false, 2*time.Minute
			buildOOMKill(c, k)
		},
	}
}

// oomUnderLimitOnPressuredNode: the same kill at 140Mi of 512Mi: the
// node ran out of memory and killed a container that stayed in bounds.
func oomUnderLimitOnPressuredNode() scenario {
	return scenario{
		expect: expectation{
			Name: "oom-under-limit-on-pressured-node",
			Description: "A container is OOM-killed far below its limit " +
				"on a node under memory pressure; the node is the cause.",
			Root: "node//n3", Tier: "notify", MaxMessages: 3,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildOOMKill(c, pressuredKill(140)) },
	}
}

func pressuredKill(usedMi float64) oomKill {
	return oomKill{
		node: "n3", victimNS: "billing", victim: "ledger",
		hogNS: "search", hog: "indexer", usedMi: usedMi,
		hogLimit: "24Gi", pressure: true,
	}
}
