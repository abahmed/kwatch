package kube

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// BootWindow is how long a node counts as booting. Staging pools scale
// from zero every morning: nodes join together, pull every image cold
// and start every pod at once, so for about this long pods that are
// pending, creating or not ready yet are expected, not failures.
const BootWindow = 10 * time.Minute

// GroupBootRemaining is how much longer a node pool or zone is booting,
// or zero when it is not. A group is booting while at least half of its
// nodes were created less than BootWindow ago: one replacement node in
// an old pool is not a boot, a pool scaled up from zero is. The group
// stops booting when the nodes created first have aged out, which is
// when most of the pool has been up for BootWindow.
func GroupBootRemaining(
	r inventory.Reader, group inventory.EntityID, now time.Time,
) time.Duration {
	if group.Name == "" || now.IsZero() {
		return 0
	}
	var ages []time.Duration
	for _, node := range r.Related(group, inventory.PartOf,
		inventory.Incoming) {
		entity, ok := r.Entity(node)
		if !ok {
			continue
		}
		attribute, ok := entity.Attribute(AttrCreated)
		if !ok || attribute.Value.AsTime().IsZero() {
			// Without a creation time the node cannot be called new.
			ages = append(ages, BootWindow)
			continue
		}
		ages = append(ages, now.Sub(attribute.Value.AsTime()))
	}
	if len(ages) == 0 {
		return 0
	}
	sort.Slice(ages, func(i, j int) bool { return ages[i] < ages[j] })
	// Booting means half or more of the nodes are young, so the group
	// is over once at most (n-1)/2 are: the node at that index decides.
	deciding := ages[(len(ages)-1)/2]
	if remaining := BootWindow - deciding; remaining > 0 {
		return remaining
	}
	return 0
}

// NodeBootRemaining is how much longer the node's pool is booting, or
// the node's zone when it names no pool. A node of neither is never
// booting: nothing says which nodes it joined with.
func NodeBootRemaining(
	r inventory.Reader, node inventory.EntityID, now time.Time,
) time.Duration {
	var longest time.Duration
	pool := false
	for _, group := range r.Related(node, inventory.PartOf,
		inventory.Outgoing) {
		if group.Kind == KindNodePool && group.Name != "" {
			pool = true
			longest = max(longest, GroupBootRemaining(r, group, now))
		}
	}
	if pool {
		return longest
	}
	for _, group := range r.Related(node, inventory.PartOf,
		inventory.Outgoing) {
		if group.Kind == KindZone {
			longest = max(longest, GroupBootRemaining(r, group, now))
		}
	}
	return longest
}

// ClusterBootRemaining is how much longer any node pool is booting: the
// time capacity for pods that no node holds yet is on its way.
func ClusterBootRemaining(r inventory.Reader, now time.Time) time.Duration {
	var longest time.Duration
	for _, pool := range r.Entities(KindNodePool) {
		longest = max(longest, GroupBootRemaining(r, pool, now))
	}
	return longest
}

// NodeYoungRemaining is how much longer the node itself counts as new:
// BootWindow after it was created, or zero. Unlike NodeBootRemaining it
// looks at this node alone, so a single replacement node in an old pool
// is young too. A node without a creation time is never young.
func NodeYoungRemaining(
	r inventory.Reader, node inventory.EntityID, now time.Time,
) time.Duration {
	entity, ok := r.Entity(node)
	if !ok || now.IsZero() {
		return 0
	}
	attribute, ok := entity.Attribute(AttrCreated)
	if !ok || attribute.Value.AsTime().IsZero() {
		return 0
	}
	return max(0, BootWindow-now.Sub(attribute.Value.AsTime()))
}
