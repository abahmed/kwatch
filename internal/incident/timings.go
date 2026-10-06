package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/format"
)

// Every duration that shapes an incident's life lives here, so the
// relations between them can be read in one place. See
// docs/incident-lifecycle/timings.md for the story they tell.
//
// How they relate:
//
//	settle   -> the wait before the first message (Config.Settle, or
//	            PageSettle for a page).
//	hold     -> how long a recovered incident must stay healthy before it
//	            resolves. It doubles for each recent recovery (the flap
//	            window counts them) up to MaxHold, and is multiplied by
//	            ChronicFactor for a chronic flapper.
//	flap     -> FlapWindow is how far back recoveries and re-openings are
//	            counted. FlapCycles of them make the incident flapping,
//	            and a flapping incident resolves only after MaxHold.
//	repage   -> RepageWindow is how soon after a page resolved the same
//	            failure may not page again. It is longer than MaxHold, so
//	            a flapping page cannot page on every cycle.
//	remind   -> an open, announced incident is said again after
//	            PageRemindAfter (first reminder of a page) and then every
//	            RemindEvery; incidents with no message of their own are
//	            said again every ChronicRemindEvery.
//	update   -> MaterialGap spaces material-change updates, FixWatch
//	            spaces the two fix-attempt updates, ReviseSettle waits for
//	            a revised cause or a reopening to collect its news.
//
// Defaults for Config. Each can be overridden in Config.
const (
	// DefaultSettle is the wait before the first message of an incident,
	// so one failure's findings arrive as one message.
	DefaultSettle = 75 * time.Second
	// DefaultPageSettle is the shorter settle of a page, which is urgent.
	DefaultPageSettle = 15 * time.Second
	// DefaultReviseSettle covers the failures that usually follow a
	// revised cause within seconds, such as evictions after pressure.
	DefaultReviseSettle = 30 * time.Second
	// DefaultHold is the base time a recovered incident stays healthy
	// before it resolves.
	DefaultHold = 3 * time.Minute
	// DefaultMaxHold caps the doubled hold, and is how long a flapping
	// incident must be stable before it resolves.
	DefaultMaxHold = 30 * time.Minute
	// DefaultFlapWindow is how far back recoveries are counted as flaps.
	DefaultFlapWindow = 30 * time.Minute
	// DefaultRemember keeps resolved incidents for a week, long enough to
	// learn daily routines and to say "3rd time this week".
	DefaultRemember = format.Week
)

// Chronic flappers. ChronicWindow and ChronicOccurrences define one: a
// root that opened at least ChronicOccurrences times (the first opening
// and one re-opening) within ChronicWindow. The resolve hold of a
// chronic flapper is multiplied by ChronicFactor, so a webhook or lease
// that fails every few minutes stops sending "healthy again" and "fails
// again" in turn: a fast flap collapses into fewer messages.
const (
	ChronicWindow      = time.Hour
	ChronicOccurrences = 2
	// ChronicFactor is two doublings: the incident was declared healthy
	// once and failed again, so a quiet spell of the usual length has
	// already proved nothing. Doubling once left the hold shorter than
	// the usual gap between failures of a flapping node (12 minutes
	// against 13), so its next "healthy" and "failing again" were still
	// sent.
	ChronicFactor = 4
)

// Reminders and repeats for incidents that stay open or keep coming back.
const (
	// PageRemindAfter is when a page-tier incident that is still open
	// is said again, once. A multi-hour outage must not go quiet after
	// its page; after this reminder the weekly cadence applies.
	PageRemindAfter = 6 * time.Hour
	// ChronicRemindEvery is how often an incident that no message of
	// its own repeats (it waits for the digest, or a roll-up named it)
	// is said again: "still failing, for 3d now". Without it such an
	// incident would be heard of once and never again.
	ChronicRemindEvery = format.Day
	// RepageWindow is how soon after a page resolved the same failure
	// may not page again: it is the same outage flapping, so it
	// notifies instead.
	RepageWindow = 2 * time.Hour
	// DigestReportEvery caps how long a periodic recurrence may stay out
	// of the digests: at least one digest a day lists it.
	DigestReportEvery = format.Day
	// RemindEvery is how often an incident that stays open is said
	// again: "still failing, for a week now".
	RemindEvery = format.Week
)

// Resolve holds.
const (
	// StillBrokenMax bounds the extra hold of a workload that is still
	// short of replicas with failing pods (stillBroken): after this long
	// without a finding the incident resolves anyway. A pod no detector
	// flags would otherwise keep it, and its paging alert, open for
	// ever. It equals RepageWindow, so a failure that returns after the
	// resolve reopens the same incident, and the coverage check hands
	// back a workload that is really down.
	StillBrokenMax = RepageWindow
)

// Updates while an incident is open.
const (
	// MaterialGap is the least time between two material-change updates
	// of one incident. A tier rise is not held back.
	MaterialGap = 10 * time.Minute
	// FixWatch is how long after a fix attempt an incident may keep
	// failing before the thread says so.
	FixWatch = 10 * time.Minute
	// HPAStuckAfter is how long an autoscaler may stay at its maximum
	// before the digest is not enough: the workload is out of headroom.
	HPAStuckAfter = 30 * time.Minute
	// RecentWindow is how far back "what changed" looks.
	RecentWindow = 30 * time.Minute
)

// Known problems and rhythms: what the incident has already told people.
const (
	// KnownAfter is how long a problem must have been heard about, from
	// its first heard occurrence, before its recurrences are known.
	KnownAfter = format.Day
	// RhythmWindow is how far back a rhythm is looked for.
	RhythmWindow = format.Day
	// routineWindow is how close to the same time of day occurrences
	// must be for an incident to be a routine.
	routineWindow = 45 * time.Minute
	// recurrenceWeek is the window of "the third time this week": the
	// timeline counts the occurrences inside it.
	recurrenceWeek = format.Week
)
