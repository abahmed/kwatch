package compose

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// movedAway reports an incident whose cause was revised, so its root is
// no longer the one it was announced for. The subject recorded at the
// announcement is then stale: the thread was told about the new root in
// the update that said the cause changed.
func movedAway(p incident.Incident) bool {
	former := p.FormerRoot()
	return !former.IsZero() && former != p.Root
}

// movedSpan is how long the failures of the root an incident moved to
// lasted. The incident's own span starts when the first root failed, and
// that failure may have ended long before: a node that came back after
// four minutes while its workload went on failing for ten. The span
// starts at the first failure that was still going when the former root
// recovered. It is false when the timeline cannot tell.
func movedSpan(p incident.Incident) (time.Duration, bool) {
	if !movedAway(p) {
		return 0, false
	}
	start := traceMove(p).ownStart()
	span := p.RecoveredAt().Sub(start)
	return span, !start.IsZero() && span > 0
}

// moveTrace is what the timeline of an incident that moved says about
// who failed and recovered when.
type moveTrace struct {
	former inventory.EntityID
	// formerBack is when the former root recovered; zero if it never did.
	formerBack time.Time
	// joined and recovered are the first failure and the last recovery
	// of every other entity.
	joined, recovered map[inventory.EntityID]time.Time
}

func traceMove(p incident.Incident) moveTrace {
	t := moveTrace{former: p.FormerRoot(),
		joined:    map[inventory.EntityID]time.Time{},
		recovered: map[inventory.EntityID]time.Time{}}
	for _, e := range p.Timeline {
		if e.Entity != nil {
			t.add(e)
		}
	}
	return t
}

func (t *moveTrace) add(e incident.Event) {
	id := *e.Entity
	switch {
	case !strings.HasPrefix(e.Text, incident.RecoveredPrefix):
		if _, ok := t.joined[id]; !ok {
			t.joined[id] = e.At
		}
	case id == t.former:
		t.formerBack = e.At
	default:
		t.recovered[id] = e.At
	}
}

// ownStart is the earliest failure of an entity other than the former
// root that was still going when the former root recovered.
func (t moveTrace) ownStart() time.Time {
	var start time.Time
	for id, at := range t.joined {
		back, done := t.recovered[id]
		if id == t.former || (done && !back.After(t.formerBack)) {
			continue
		}
		if start.IsZero() || at.Before(start) {
			start = at
		}
	}
	return start
}
