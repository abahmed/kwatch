package incident

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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

// RecentChanges returns the changes worth naming in root's namespace
// during the RecentWindow before now, newest first, at most
// MaxRecentChanges, one per object. Without a model, or for a root that
// lives outside a namespace, it returns nothing.
func RecentChanges(
	model inventory.HistoryReader, root inventory.EntityID, now time.Time,
) []inventory.Change {
	if model == nil || root.Namespace == "" {
		return nil
	}
	newest := newestPerObject(model, root.Namespace, now)
	out := make([]inventory.Change, 0, len(newest))
	for _, change := range newest {
		out = append(out, change)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].Entity.String() < out[j].Entity.String()
	})
	if len(out) > MaxRecentChanges {
		out = out[:MaxRecentChanges]
	}
	return out
}

// worthNaming reports a change a person made in namespace within the
// window: not noise, not a status write, not an autoscaler's replicas.
func worthNaming(
	change inventory.Change, namespace string, now time.Time,
) bool {
	switch {
	case change.Entity.Namespace != namespace,
		noisyKinds[change.Entity.Kind],
		change.At.Before(now.Add(-RecentWindow)), change.At.After(now),
		strings.Contains(strings.ToLower(change.Actor), "kwatch"):
		return false
	case change.Created || change.Deleted:
		return true
	}
	// A change without field edits is a status write.
	return len(change.Fields) > 0 && change.Classify() != inventory.ClassScale
}

// newestPerObject keeps, for each object of namespace, its newest change
// worth naming in the window before now.
func newestPerObject(
	model inventory.HistoryReader, namespace string, now time.Time,
) map[inventory.EntityID]inventory.Change {
	newest := map[inventory.EntityID]inventory.Change{}
	for _, set := range model.RecentChangeSets(now.Add(-RecentWindow)) {
		for _, change := range set.Changes {
			if !worthNaming(change, namespace, now) {
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
