package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultWindow is how far before Now a change still counts as a
// possible cause. Rollouts and config edits usually break things within
// minutes; half an hour also covers slow crash loops.
const DefaultWindow = 30 * time.Minute

// Snapshot is everything Explain reads. It is a value: the caller
// builds it once per solve and Explain never changes it.
type Snapshot struct {
	// Model is the inventory graph.
	Model inventory.Reader
	// Findings holds the active findings of each entity. Every entity
	// with a Failing or Degraded finding is a failure to explain.
	Findings map[inventory.EntityID][]detection.Finding
	// Links resolves links computed at read time, such as selects and
	// managed-by. Nil means only stored relations are followed.
	Links LinkReader
	// Changes reports meaningful changes. Nil means no change data:
	// no candidate is kept for having changed.
	Changes ChangeReader
	// Outcomes judges how the changes of a candidate turned out. Nil
	// says nothing.
	Outcomes OutcomeReader
	// Baseline reports how unusual an entity looks. Nil is neutral.
	Baseline BaselineReader
	// Synced reports whether a kind is fully watched, so an absent
	// object of that kind is really missing. Nil means unknown.
	Synced func(inventory.Kind) bool
	// Verifiable reports whether kwatch can observe a kind at all. An
	// entity of a kind it cannot see is never blamed; the area notes
	// the gap in Unverified instead. Nil treats every kind as visible.
	Verifiable func(inventory.Kind) bool
	// Focus limits the failures to explain. Nil explains every failure
	// of Findings. The other findings are still read as causes.
	Focus []inventory.EntityID
	// Now is the time of the snapshot. Explain never reads a clock.
	Now time.Time
	// Window is the causal window. Zero means DefaultWindow.
	Window time.Duration
}

// window returns the causal window with its default applied.
func (s Snapshot) window() time.Duration {
	if s.Window <= 0 {
		return DefaultWindow
	}
	return s.Window
}

// verifiable reports whether kind can be observed.
func (s Snapshot) verifiable(kind inventory.Kind) bool {
	return s.Verifiable == nil || s.Verifiable(kind)
}

// synced reports whether kind is known to be fully watched.
func (s Snapshot) synced(kind inventory.Kind) bool {
	return s.Synced != nil && s.Synced(kind)
}

// Link is one link resolved at read time, from an entity to a target.
type Link struct {
	Type inventory.RelationType
	To   inventory.EntityID
	// Inferred marks a link derived from a naming convention.
	Inferred bool
}

// LinkReader resolves the links that are not stored as relations.
type LinkReader interface {
	Links(id inventory.EntityID) []Link
}

// ChangeReader lists the meaningful changes of an entity between since
// and until. The change model is provided by the inventory; this
// interface keeps the engine testable without it.
type ChangeReader interface {
	Changes(id inventory.EntityID, since, until time.Time) []inventory.Change
}

// BaselineReader reports how far an entity's current behaviour is from
// its learned baseline: 0 is normal, 1 is as unusual as it gets. ok is
// false when there is no baseline yet.
type BaselineReader interface {
	Deviation(id inventory.EntityID) (deviation float64, ok bool)
}

// OutcomeReader judges the change set a change belongs to as of now:
// healthy, degraded, reverted or still pending. ok is false when the
// change belongs to no known set.
type OutcomeReader interface {
	Outcome(change inventory.Change, now time.Time) (
		outcome inventory.Outcome, ok bool)
}

// KubeLinks resolves read-time links with kube.Links.
type KubeLinks struct {
	Reader inventory.Reader
}

// Links implements LinkReader.
func (k KubeLinks) Links(id inventory.EntityID) []Link {
	resolved := kube.Links(k.Reader, id)
	out := make([]Link, 0, len(resolved))
	for _, link := range resolved {
		out = append(out, Link{
			Type: link.Type, To: link.To, Inferred: link.Inferred,
		})
	}
	return out
}

// ModelChanges reads changes from the inventory's own change ring.
type ModelChanges struct {
	Reader inventory.Reader
}

// Changes implements ChangeReader.
func (m ModelChanges) Changes(
	id inventory.EntityID, since, until time.Time,
) []inventory.Change {
	var out []inventory.Change
	for _, change := range m.Reader.Changes(id, since) {
		if !change.At.After(until) {
			out = append(out, change)
		}
	}
	return out
}
