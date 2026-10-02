package inventory

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// ChangeSetWindow is how close in time two related changes must be to
// belong to one change set.
const ChangeSetWindow = 2 * time.Minute

// maxOwnerDepth bounds the owner chain followed when deciding whether
// two entities are related: pod, ReplicaSet, Deployment.
const maxOwnerDepth = 3

// maxLabelItems bounds the items listed in a change set label.
const maxLabelItems = 3

// ChangeSet is changes close in time on related objects: one release,
// evaluated as one cause.
type ChangeSet struct {
	// ID is stable: it is assigned once when the set is created, from
	// its first change. Later changes that join the set, a merge with a
	// younger set, or the first change leaving the history never change
	// it.
	ID string
	// Aliases are the IDs of younger sets merged into this one. Timeline
	// entries written before a merge name the younger ID; ChangeSet
	// resolves it through these.
	Aliases []string `json:",omitempty"`
	// Start and End are the times of the first and last change.
	Start, End time.Time
	// Who is the GitOps application, or else the actor, of the set.
	Who string
	// Label is a short human description, such as
	// "14:02 release by argocd/shop: image v2.3, ConfigMap app-config".
	Label string
	// Changes are the members, oldest first.
	Changes []Change
}

// Entities returns the entities the set changed, each once, in order of
// their first change.
func (s ChangeSet) Entities() []EntityID {
	var out []EntityID
	for _, change := range s.Changes {
		if !containsID(out, change.Entity) {
			out = append(out, change.Entity)
		}
	}
	return out
}

// ChangeSets returns the change sets that changed id or an entity related
// to it (its owners and the objects they reference) and ended at or
// after since, oldest first.
func (m *Model) ChangeSets(id EntityID, since time.Time) []ChangeSet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	target := m.scopeLocked(id)
	var out []ChangeSet
	for _, set := range m.changeSetsLocked() {
		if set.End.Before(since) {
			continue
		}
		for _, entity := range set.Entities() {
			if target.overlaps(m.scopeLocked(entity)) {
				out = append(out, cloneSet(set))
				break
			}
		}
	}
	return out
}

// RecentChangeSets returns every change set that ended at or after since,
// oldest first.
func (m *Model) RecentChangeSets(since time.Time) []ChangeSet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ChangeSet
	for _, set := range m.changeSetsLocked() {
		if !set.End.Before(since) {
			out = append(out, cloneSet(set))
		}
	}
	return out
}

// ChangeSet returns the change set with id, also when id names a set
// that was merged into another one since: a persisted timeline entry
// keeps the ID its set had when the entry was written.
func (m *Model) ChangeSet(id string) (ChangeSet, bool) {
	if id == "" {
		return ChangeSet{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, set := range m.changeSetsLocked() {
		if set.ID == id || containsText(set.Aliases, id) {
			return cloneSet(set), true
		}
	}
	return ChangeSet{}, false
}

// linked reports whether two changes belong to one release: related
// objects, the same GitOps application, or the same person in the same
// namespace.
func linked(a, b Change, scopeOf func(EntityID) entityScope) bool {
	if a.App != "" && a.App == b.App {
		return true
	}
	if personal(a.Actor) && a.Actor == b.Actor &&
		a.Entity.Namespace == b.Entity.Namespace {
		return true
	}
	return scopeOf(a.Entity).overlaps(scopeOf(b.Entity))
}

// personal reports an actor that stands for a person or a deploy tool.
// Built-in controllers write on behalf of every object, so sharing one
// says nothing about a release.
func personal(actor string) bool {
	return actor != "" && !strings.HasPrefix(actor, "kube-") &&
		!strings.HasPrefix(actor, "kubelet")
}

func setWho(changes []Change) string {
	for _, change := range changes {
		if change.App != "" {
			return change.App
		}
	}
	for _, change := range changes {
		if personal(change.Actor) {
			return change.Actor
		}
	}
	if changes[0].Actor != "" {
		return changes[0].Actor
	}
	return "unknown"
}

// setLabel renders "14:02 release by argocd/shop: image v2.3, ConfigMap
// app-config". Items are ordered by importance and listed once.
func setLabel(set ChangeSet) string {
	ordered := append([]Change(nil), set.Changes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return labelRank(ordered[i]) > labelRank(ordered[j])
	})
	var items []string
	for _, change := range ordered {
		item := describeChange(change)
		if item != "" && !containsText(items, item) {
			items = append(items, item)
		}
	}
	if extra := len(items) - maxLabelItems; extra > 0 {
		items = append(items[:maxLabelItems], "+"+strconv.Itoa(extra)+" more")
	}
	head := set.Start.UTC().Format("15:04") + " release by " + set.Who
	if len(items) == 0 {
		return head
	}
	return head + ": " + strings.Join(items, ", ")
}

func labelRank(change Change) int {
	rank := classRank(change.Classify())
	if change.Entity.Kind == "replicaset" || change.Entity.Kind == "pod" {
		// Controllers scale ReplicaSets and replace pods during every
		// rollout; the workload's own change says more.
		rank -= 10
	}
	return rank
}

func containsText(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func cloneSet(set ChangeSet) ChangeSet {
	changes := make([]Change, len(set.Changes))
	for i, change := range set.Changes {
		changes[i] = cloneChange(change)
	}
	set.Changes = changes
	set.Aliases = append([]string(nil), set.Aliases...)
	return set
}
