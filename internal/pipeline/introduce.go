package pipeline

import (
	"context"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// A roll-up or a namespace outage names the incidents it carries and
// says that they fail, not why: each one's first own message introduces
// it in full. Nothing guarantees that message, though. Updates go out
// only when an incident changes materially, and a crash loop that just
// goes on never does, so the cause and what the checks read could stay
// untold for good. So each listed incident gets one introduction of its
// own once its investigation is in, threaded under the listing (it is
// written as an announcement, see announce.Listing.follow): never
// twice, never to the pagers, and never when it already spoke on its
// own. An incident a namespace outage holds waits for that listing to
// go out. An incident without an investigation, or whose investigation
// or listing is not in within outputWait, is not introduced: the
// listing and its later updates are what people get, as before.

// noteListed remembers the announcements in before that a listing took
// (they are missing from after), to introduce once their evidence is in.
func (a *announcer) noteListed(
	before, after []incident.Decision, now time.Time,
) {
	if a.pool == nil {
		return
	}
	left := map[string]bool{}
	for _, d := range after {
		left[d.Incident.ID] = true
	}
	for _, d := range before {
		if d.Action != incident.Announce || left[d.Incident.ID] {
			continue
		}
		if a.intros == nil {
			a.intros = map[string]time.Time{}
		}
		a.intros[d.Incident.ID] = now.Add(outputWait)
	}
}

// queueRestoredIntros starts the investigation of each restored
// incident still owed its introduction (announce restored_intro.go) and
// waits for it like a listed one.
func (a *announcer) queueRestoredIntros(now time.Time) {
	owed := a.collect.TakeRestoredIntros()
	if a.pool == nil {
		return
	}
	for _, p := range owed {
		if _, waiting := a.intros[p.ID]; waiting {
			continue
		}
		a.investigate(p, now)
		if a.intros == nil {
			a.intros = map[string]time.Time{}
		}
		a.intros[p.ID] = now.Add(outputWait)
	}
}

// introduceDue sends the introductions whose investigation is in and
// drops those that waited outputWait in vain. It reports whether it
// sent one, which changes the incident's saved state.
func (a *announcer) introduceDue(ctx context.Context, now time.Time) bool {
	ids := make([]string, 0, len(a.intros))
	for id := range a.intros {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	sent := false
	for _, id := range ids {
		state := a.evidence[id]
		ready := state != nil && !state.running
		waiting := now.Before(a.intros[id])
		if !ready && waiting {
			continue
		}
		if ready && waiting && !a.collect.SpokeOnItsOwn(id) &&
			a.incidents.AwaitsListing(id) {
			// A namespace outage still holds it: its listing, and so
			// the roll-up mark Introduce needs, comes later.
			continue
		}
		delete(a.intros, id)
		if !ready || a.collect.SpokeOnItsOwn(id) {
			continue
		}
		d, ok := a.incidents.Introduce(id)
		if !ok {
			continue
		}
		contain("compose", "write", func() {
			a.send(ctx, a.withEvidence(d), now)
		})
		sent = true
	}
	return sent
}
