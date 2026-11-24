package announce

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// An ongoing problem is listed in full at most once a day: in the first
// digest of the day that names it, or in a later one when it changed.
// Every other digest says "N more still failing, unchanged (a, b, c +K)"
// instead of repeating the same lines. A problem that began more than
// WeeklyAfter ago and did not change is listed in full at most once a
// week (ISO week, UTC): the first digest of the week lists it with the
// others still open, and the other digests only count it. The audit log
// still lists them all.

// WeeklyAfter is how long a problem must have lasted before an
// unchanged one is listed in full once a week instead of once a day.
const WeeklyAfter = 7 * 24 * time.Hour

// shown is how a problem was last listed in full.
type shown struct {
	// day is the UTC date of the digest, or its ISO week for a problem
	// listed weekly (periodOf); sig is what it said (see
	// ongoingSignature).
	day, sig string
}

// ongoingSignature is what a digest says about a problem apart from how
// long it has lasted: whether it flaps, its reason, how many things fail
// and its cause. A change in any of them is news. A problem whose
// findings are all gone is about to resolve: its signature is empty and
// matches whatever it was listed as.
func ongoingSignature(p incident.Incident) string {
	failing := 0
	for _, f := range p.Members {
		if !f.Advisory {
			failing++
		}
	}
	if failing == 0 {
		return ""
	}
	sig := strconv.FormatBool(p.State == incident.Flapping) + "/" +
		ongoingReason(p) + "/" + strconv.Itoa(failing)
	if p.Cause != nil {
		sig += "/" + p.Cause.Root.String() + "/" + p.Cause.Rule + "/" +
			string(p.Cause.Mode)
	}
	return sig
}

func dayOf(now time.Time) string { return now.UTC().Format("2006-01-02") }

// weekOf is the ISO week of now, UTC: "2026-W48".
func weekOf(now time.Time) string {
	year, week := now.UTC().ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// periodOf is how often item is listed in full: the day, or the week
// for a problem that began more than WeeklyAfter ago.
func periodOf(item ongoingItem, now time.Time) string {
	if !item.Since.IsZero() && now.Sub(item.Since) > WeeklyAfter {
		return weekOf(now)
	}
	return dayOf(now)
}

// splitOngoing separates the problems to list in full from the ones
// this day's digests already listed and that did not change.
func (c *Collector) splitOngoing(
	items []ongoingItem, now time.Time,
) (changed, unchanged []ongoingItem) {
	for _, item := range items {
		last, ok := c.ongoingShown[item.p.ID]
		sig := ongoingSignature(item.p)
		if ok && last.day == periodOf(item, now) &&
			(sig == "" || last.sig == sig) {
			unchanged = append(unchanged, item)
			continue
		}
		changed = append(changed, item)
	}
	return changed, unchanged
}

// markShown remembers that a digest listed the problem in full, in
// its period (periodOf). Records of a past day or week are dropped.
func (c *Collector) markShown(item ongoingItem, now time.Time) {
	if c.ongoingShown == nil {
		c.ongoingShown = map[string]shown{}
	}
	today, week := dayOf(now), weekOf(now)
	for id, last := range c.ongoingShown {
		if last.day != today && last.day != week {
			delete(c.ongoingShown, id)
		}
	}
	c.ongoingShown[item.p.ID] = shown{day: periodOf(item, now),
		sig: ongoingSignature(item.p)}
}

// ongoingExtras are the digest's ongoing problems: in full, and as the
// summary line.
func ongoingExtras(
	changed, unchanged []ongoingItem,
) (full, same []compose.Ongoing) {
	for _, item := range changed {
		full = append(full, item.Ongoing)
	}
	for _, item := range unchanged {
		same = append(same, item.Ongoing)
	}
	return full, same
}

// shownLines is the record of what this day's and week's digests
// listed, in ID order, for the saved digest state: a restart must not
// make every ongoing problem new again.
func (c *Collector) shownLines() []ShownLine {
	var out []ShownLine
	for _, id := range slices.Sorted(maps.Keys(c.ongoingShown)) {
		last := c.ongoingShown[id]
		out = append(out, ShownLine{ID: id, Day: last.day, Sig: last.sig})
	}
	return out
}

// restoreShown reads the record back after a restart.
func (c *Collector) restoreShown(lines []ShownLine) {
	if len(lines) == 0 {
		return
	}
	c.ongoingShown = map[string]shown{}
	for _, l := range lines {
		c.ongoingShown[l.ID] = shown{day: l.Day, sig: l.Sig}
	}
}
