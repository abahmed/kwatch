package announce

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// RestoredRelist is how long the same restart note is not sent again,
// and, for a listing saved before the told digests (Told), how long its
// incidents are not listed again: a kwatch that restarts every few
// minutes would otherwise post the same list each time.
const RestoredRelist = time.Hour

// RestoredRetell is how long after a listing named an incident a later
// restart may leave it out while it is unchanged: restarts close
// together repeat nothing, but a cluster that sleeps every night still
// names, each morning, what is still failing.
const RestoredRetell = 12 * time.Hour

// ToldMark is an incident's told fingerprint and when a listing named
// it.
type ToldMark struct {
	Digest string
	At     time.Time
}

// RestoredMark is the persisted memory of the restored listings: which
// incidents were listed and when, and the last restart note.
type RestoredMark struct {
	// At is when IDs were last listed.
	At  time.Time
	IDs []string `json:",omitempty"`
	// Told is the told fingerprint of each incident a listing named,
	// kept while it stays restored and failing: a later restart lists
	// it again only when it changed since or RestoredRetell passed
	// (toldUnchanged). Absent in files written before it; their IDs
	// keep the RestoredRelist rule.
	Told map[string]ToldMark `json:",omitempty"`
	// NoteAt is when the last restart note was sent and NoteKey what
	// it said (restart reason and incidents), so the same note is not
	// sent twice within RestoredRelist.
	NoteAt  time.Time `json:",omitzero"`
	NoteKey string    `json:",omitempty"`
}

func (m RestoredMark) clone() RestoredMark {
	m.IDs = slices.Clone(m.IDs)
	m.Told = maps.Clone(m.Told)
	return m
}

// Restart is how this start followed the previous session, for the
// restart note. The zero value says nothing about it.
type Restart struct {
	// At is when kwatch started.
	At time.Time
	// Reason is the restart reason code of the previous session's end
	// (graceful_shutdown, leader_handoff, api_unavailable, oom_killed,
	// eviction, node_disruption, deployment_rollout, internal_failure
	// or unknown); empty when unknown or on a first run.
	Reason string
	// Down is how long nothing was monitored before this start.
	Down time.Duration
}

// toldUnchanged reports a restored incident a listing named less than
// RestoredRetell ago and that has not changed since: its told
// fingerprint is the one the listing saw. Listing it after every
// restart would only repeat the same list; the short restart note says
// it is still open instead.
// Without a told digest on either side (a listing saved before them, an
// incident never fingerprinted) the old rule holds: named within
// RestoredRelist of now.
func (c *Collector) toldUnchanged(p incident.Incident, now time.Time) bool {
	m := c.Startup.Summary.Restored
	if m == nil {
		return false
	}
	if t, ok := m.Told[p.ID]; ok && t.Digest != "" && p.Digest != "" {
		return t.Digest == p.Digest && now.Sub(t.At) < RestoredRetell
	}
	return now.Sub(m.At) < RestoredRelist && slices.Contains(m.IDs, p.ID)
}

// rememberListed records that the incidents of listed were listed at
// now, keeping the ones a listing within RestoredRelist named, and
// saves the marker.
func (c *Collector) rememberListed(
	listed []incident.Decision, now time.Time,
) {
	s := &c.Startup.Summary
	mark := RestoredMark{At: now, Told: map[string]ToldMark{}}
	if s.Restored != nil {
		mark.NoteAt, mark.NoteKey = s.Restored.NoteAt, s.Restored.NoteKey
		if now.Sub(s.Restored.At) < RestoredRelist {
			mark.IDs = slices.Clone(s.Restored.IDs)
		}
		// Keep the marks of incidents still restored and failing;
		// the others resolved or are gone, so the map stays bounded.
		live := map[string]bool{}
		for _, p := range c.env.Incidents.RestoredFailing() {
			live[p.ID] = true
		}
		for id, t := range s.Restored.Told {
			if live[id] {
				mark.Told[id] = t
			}
		}
	}
	for _, d := range listed {
		if !slices.Contains(mark.IDs, d.Incident.ID) {
			mark.IDs = append(mark.IDs, d.Incident.ID)
		}
		mark.Told[d.Incident.ID] = ToldMark{
			Digest: d.Incident.Digest, At: now}
	}
	s.Restored = &mark
	c.saveSummary()
}

// saveSummary hands the startup marker to the writer, when there is one.
func (c *Collector) saveSummary() {
	if c.env.SaveStartup != nil {
		c.env.SaveStartup(c.Startup.Summary)
	}
}

// sendRestartNote replaces the restored listing when every restored
// incident still failing was listed after a recent restart: one short
// message says kwatch restarted, why, and how many are still open. The
// same note (same reason, same incidents) is not sent twice within
// RestoredRelist. It routes like the listing it replaces.
func (c *Collector) sendRestartNote(
	ctx context.Context, now time.Time, open []incident.Decision,
) bool {
	if len(open) == 0 {
		return false
	}
	s := &c.Startup.Summary
	key := noteKey(c.env.Restart, open)
	if m := s.Restored; m != nil && m.NoteKey == key &&
		now.Sub(m.NoteAt) < RestoredRelist {
		return false
	}
	msg := c.env.Messages.RestoredSummaryWith(open, 0, now)
	text := restartNoteText(c.env.Restart, len(open), now,
		c.dashboardIssues())
	msg.Title, msg.Short = text, msg.Marker+" "+text
	msg.Note, msg.Lines = msg.Marker+" "+text, nil
	msg.Doc = []notification.Block{{Kind: notification.Para,
		Spans: []notification.Span{{Text: text}}}}
	msg.Listed = restoredListing(open)
	klog.InfoS("pipeline: restart note", "component", "pipeline",
		"open", len(open), "reason", c.env.Restart.Reason)
	c.env.Sink(ctx, incident.Decision{Reason: "restored incidents"}, msg)
	mark := RestoredMark{}
	if s.Restored != nil {
		mark = s.Restored.clone()
	}
	mark.NoteAt, mark.NoteKey = now, key
	s.Restored = &mark
	c.saveSummary()
	return true
}

// dashboardIssues is the dashboard's issue list, "" without one.
func (c *Collector) dashboardIssues() string {
	if c.env.Messages.Dashboard == nil {
		return ""
	}
	if base := c.env.Messages.Dashboard(); base != "" {
		return base + "/issues"
	}
	return ""
}

// noteKey is what a restart note says: the restart reason and the
// incidents still open, sorted.
func noteKey(r Restart, open []incident.Decision) string {
	ids := make([]string, 0, len(open))
	for _, d := range open {
		ids = append(ids, d.Incident.ID)
	}
	slices.Sort(ids)
	return r.Reason + "|" + strings.Join(ids, ",")
}

// restartNoteText words the restart note, such as "kwatch restarted at
// 10:02 UTC (it had lost the API server, down 40s). 3 problems from
// before are still open; see the dashboard: <address>."
func restartNoteText(
	r Restart, open int, now time.Time, issues string,
) string {
	at := r.At
	if at.IsZero() {
		at = now
	}
	text := "kwatch restarted at " + at.UTC().Format("15:04") + " UTC"
	why := restartWhy(r.Reason)
	if r.Down > 0 {
		down := "down " + format.Duration(r.Down)
		if why == "" {
			why = down
		} else {
			why += ", " + down
		}
	}
	if why != "" {
		text += " (" + why + ")"
	}
	problems := strconv.Itoa(open) + " problems from before are"
	if open == 1 {
		problems = "1 problem from before is"
	}
	text += ". " + problems + " still open"
	if issues != "" {
		return text + "; see the dashboard: " + issues
	}
	return text + ", as listed after an earlier restart."
}

// restartWhys word the restart reason codes.
var restartWhys = map[string]string{
	"graceful_shutdown":  "it was stopped",
	"leader_handoff":     "it lost its leader Lease",
	"api_unavailable":    "it had lost the API server",
	"oom_killed":         "it ran out of memory",
	"eviction":           "it was evicted",
	"node_disruption":    "its node went away",
	"deployment_rollout": "a rollout replaced it",
	"internal_failure":   "it stopped unexpectedly",
	"unknown":            "it stopped without a stop record",
}

func restartWhy(reason string) string {
	return restartWhys[reason]
}
