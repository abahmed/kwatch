package incident

import (
	"slices"
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
		isBlamed := blamed != nil && (inventory.SameChange(change,
			*blamed) || sameEdit(change, *blamed))
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
	return rankedUpTo(model, root, from, to, below, MaxRecentChanges)
}

// rankedUpTo is rankedBetween keeping at most limit changes.
func rankedUpTo(
	model inventory.HistoryReader, root inventory.EntityID,
	from, to time.Time, below, limit int,
) []inventory.Change {
	if model == nil || root.Namespace == "" {
		return nil
	}
	near := newProximity(model, root)
	ranks := map[changeKey]int{}
	var out []inventory.Change
	for key, change := range newestPerObject(model, from, to) {
		if ownScheduledRun(model, root, change) {
			continue
		}
		if rank := near.changeRank(change); rank < below {
			ranks[key] = rank
			out = append(out, change)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := ranks[keyOf(a)], ranks[keyOf(b)]; ra != rb {
			return ra < rb
		}
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		if a.Entity != b.Entity {
			return a.Entity.String() < b.Entity.String()
		}
		return keyOf(a).link < keyOf(b).link
	})
	out = foldEndedLinks(out)
	if len(out) > limit {
		out = out[:limit]
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
// from from to to, and apart from it the end of each of its links
// (keyOf): a selector edit and the routing it ended are both named.
func newestPerObject(
	model inventory.HistoryReader, from, to time.Time,
) map[changeKey]inventory.Change {
	newest := map[changeKey]inventory.Change{}
	for _, set := range model.RecentChangeSets(from) {
		for _, change := range set.Changes {
			if !worthNaming(change, from, to) {
				continue
			}
			key := keyOf(change)
			old, seen := newest[key]
			if !seen || change.At.After(old.At) {
				newest[key] = change
			}
		}
	}
	return newest
}

// changeKey is the object a change is of and, for the end of a link,
// the relation and its target.
type changeKey struct {
	id   inventory.EntityID
	link string
}

func keyOf(change inventory.Change) changeKey {
	key := changeKey{id: change.Entity}
	if rel, target, ok := change.RemovedLink(); ok {
		key.link = string(rel) + " " + target.String()
	}
	return key
}

// changeRank is how near a change is to the root: that of its object,
// or, for the end of a link, of the object the link pointed to when
// that is nearer. A Service that stopped routing to the root's workload
// fronted it until then.
func (p *proximity) changeRank(change inventory.Change) int {
	rank := p.rank(change.Entity)
	_, target, ok := change.RemovedLink()
	if !ok {
		return rank
	}
	if p.anchors[target] || p.targets[target] ||
		target.Kind == kube.KindService && p.fronts(target, 1) {
		return min(rank, nearTraffic)
	}
	return rank
}

// Onset is when the incident's failures began: the earliest start among
// its members, else when the incident opened. Zero when neither is known.
// It is never after the opening: members replaced since (a recreated
// pod starts its own failure later) do not move the start forward.
func (p Incident) Onset() time.Time {
	var first time.Time
	for _, member := range p.Members {
		if !member.Advisory && !member.Since.IsZero() &&
			(first.IsZero() || member.Since.Before(first)) {
			first = member.Since
		}
	}
	if first.IsZero() || (!p.Opened.IsZero() && p.Opened.Before(first)) {
		return p.Opened
	}
	return first
}

// sameEdit reports a and b as one edit told twice: an edit of an
// object and the end of a link that edit brought about, made within
// kube.LinkFlapWindow of each other. Two ends of links, or two edits,
// are told apart.
func sameEdit(a, b inventory.Change) bool {
	_, _, aEnded := a.RemovedLink()
	_, _, bEnded := b.RemovedLink()
	gap := a.At.Sub(b.At).Abs()
	return a.Entity == b.Entity && aEnded != bEnded &&
		gap <= kube.LinkFlapWindow
}

// foldEndedLinks keeps, of an edit and the end of a link it brought
// about (sameEdit), the one listed first: changes come nearest to the
// root first, so a Service edit that stopped routing to the failing
// workload is told as that, and an edit of the root itself as its edit.
func foldEndedLinks(changes []inventory.Change) []inventory.Change {
	var out []inventory.Change
	for _, change := range changes {
		if !slices.ContainsFunc(out, func(kept inventory.Change) bool {
			return sameEdit(kept, change)
		}) {
			out = append(out, change)
		}
	}
	return out
}
