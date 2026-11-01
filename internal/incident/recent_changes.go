package incident

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Recent changes are what a message says when it can name no cause:
// the few things people changed next to the failure lately.
const (
	// MaxRecentChanges bounds how many changes a message names.
	MaxRecentChanges = 3
)

// noisyKinds change all the time on their own and say nothing about a
// failure: controllers renew leases and rewrite endpoint slices, and the
// pods, replica sets and events of a rollout follow the workload change
// that is already listed.
var noisyKinds = map[inventory.Kind]bool{
	kube.KindLease:         true,
	kube.KindEndpointSlice: true,
	kube.KindPod:           true,
	kube.KindReplicaSet:    true,
	kube.KindContainer:     true,
	"endpoints":            true,
	"event":                true,
}

// RecentChanges returns the changes worth naming near root during the
// RecentWindow before now, nearest to the root first and newest first
// within one distance, at most MaxRecentChanges, one per object. Nodes
// the root's pods run on count; other cluster-scoped objects do not.
// Without a model, or for a root that lives outside a namespace, it
// returns nothing.
func RecentChanges(
	model inventory.HistoryReader, root inventory.EntityID, now time.Time,
) []inventory.Change {
	return rankedChanges(model, root, now, farAway)
}

// NearChanges is RecentChanges for an incident that has a cause: only
// what sits within graph reach of the root (the object itself, what it
// uses, its Service or Ingress, its node), made before the failure began
// at onset, and never the change already blamed.
func NearChanges(
	model inventory.HistoryReader, root inventory.EntityID, now time.Time,
	onset time.Time, blamed *inventory.Change,
) []inventory.Change {
	var out []inventory.Change
	for _, change := range rankedChanges(model, root, now, nearNamespace) {
		isBlamed := blamed != nil && change.Entity == blamed.Entity &&
			change.At.Equal(blamed.At)
		late := !onset.IsZero() && change.At.After(onset.Add(onsetSlack))
		if isBlamed || late {
			continue
		}
		out = append(out, change)
	}
	return out
}

// onsetSlack absorbs the clock difference between an API server's change
// time and a detector's first sight of the failure.
const onsetSlack = time.Minute

func rankedChanges(
	model inventory.HistoryReader, root inventory.EntityID, now time.Time,
	below int,
) []inventory.Change {
	return rankedBetween(model, root, now.Add(-RecentWindow), now, below)
}

// rankedBetween is rankedChanges for the changes made from from to to.
func rankedBetween(
	model inventory.HistoryReader, root inventory.EntityID,
	from, to time.Time, below int,
) []inventory.Change {
	if model == nil || root.Namespace == "" {
		return nil
	}
	near := newProximity(model, root)
	ranks := map[inventory.EntityID]int{}
	var out []inventory.Change
	for id, change := range newestPerObject(model, from, to) {
		if ownScheduledRun(model, root, change) {
			continue
		}
		if rank := near.rank(id); rank < below {
			ranks[id] = rank
			out = append(out, change)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ranks[a.Entity] != ranks[b.Entity] {
			return ranks[a.Entity] < ranks[b.Entity]
		}
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		return a.Entity.String() < b.Entity.String()
	})
	if len(out) > MaxRecentChanges {
		out = out[:MaxRecentChanges]
	}
	return out
}

// ownScheduledRun reports the creation of a Job by the schedule of the
// CronJob root: what the root does on its own, which no one changed and
// which says nothing next to its failure.
func ownScheduledRun(
	model inventory.Reader, root inventory.EntityID, change inventory.Change,
) bool {
	return scheduledRun(root, &change) &&
		rootcause.TopOwner(model, change.Entity) == root
}

// worthNaming reports a change a person made from from to to: not
// noise, not a status write, not an autoscaler's replicas.
func worthNaming(change inventory.Change, from, to time.Time) bool {
	switch {
	case noisyKinds[change.Entity.Kind],
		change.At.Before(from), change.At.After(to),
		strings.Contains(strings.ToLower(change.Actor), "kwatch"):
		return false
	case change.Entity.Kind == kube.KindNode:
		// A node joining or leaving is capacity, not an edit.
		return len(change.Fields) > 0 && !change.Created && !change.Deleted
	case change.Created && kube.HarmlessToCreate(change.Entity.Kind):
		return false
	case change.Created || change.Deleted:
		return true
	}
	// A change without field edits is a status write.
	return len(change.Fields) > 0 && change.Classify() != inventory.ClassScale
}

// newestPerObject keeps, for each object, its newest change worth naming
// from from to to.
func newestPerObject(
	model inventory.HistoryReader, from, to time.Time,
) map[inventory.EntityID]inventory.Change {
	newest := map[inventory.EntityID]inventory.Change{}
	for _, set := range model.RecentChangeSets(from) {
		for _, change := range set.Changes {
			if !worthNaming(change, from, to) {
				continue
			}
			old, seen := newest[change.Entity]
			if !seen || change.At.After(old.At) {
				newest[change.Entity] = change
			}
		}
	}
	return newest
}

// Onset is when the incident's failures began: the earliest start among
// its members, else when the incident opened. Zero when neither is known.
func (p Incident) Onset() time.Time {
	var first time.Time
	for _, member := range p.Members {
		if !member.Advisory && !member.Since.IsZero() &&
			(first.IsZero() || member.Since.Before(first)) {
			first = member.Since
		}
	}
	if first.IsZero() {
		return p.Opened
	}
	return first
}
