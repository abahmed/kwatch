package announce

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// CarrierQuietRestart is the carrier the audit log names for a restart
// that told people nothing: no gap worth reporting and nothing new or
// worse among the restored incidents.
const CarrierQuietRestart = "quiet restart"

// ToldMark is an incident's told fingerprint and when a restored
// listing named it.
type ToldMark struct {
	Digest string
	At     time.Time
}

// RestoredMark is the persisted memory of the restored listings: which
// incidents were named and when.
type RestoredMark struct {
	// At is when IDs were last listed.
	At  time.Time
	IDs []string `json:",omitempty"`
	// Told is the told fingerprint of each incident a listing named,
	// kept while it stays restored and failing (toldBefore).
	Told map[string]ToldMark `json:",omitempty"`
	// NoteAt and NoteKey are kept from files of versions that sent a
	// separate restart note; nothing reads them.
	NoteAt  time.Time `json:",omitzero"`
	NoteKey string    `json:",omitempty"`
}

func (m RestoredMark) clone() RestoredMark {
	m.IDs = slices.Clone(m.IDs)
	m.Told = maps.Clone(m.Told)
	return m
}

// Restart is how this start followed the previous session. The zero
// value says nothing about it.
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

// HoldGap keeps the gap report for the one restart message, and
// reports whether it did: only while the restored incidents are not
// listed yet. Otherwise the caller sends it on its own at once.
func (c *Collector) HoldGap(report string) bool {
	if c.Startup.WarmAt.IsZero() || c.env.Notify == nil {
		return false
	}
	c.gap = report
	return true
}

// sendRestart sends the one message of a restart: the restored news
// with the gap and the count of unchanged ongoing incidents, or, without
// news, the gap message with that count. With neither, the restart is
// only recorded in the audit log. It reports whether it sent a message.
func (c *Collector) sendRestart(
	ctx context.Context, now time.Time, news, ongoing []incident.Decision,
) bool {
	gap := c.gap
	c.gap = ""
	count := ongoingCount(len(ongoing), c.dashboardIssues())
	switch {
	case len(news) > 0:
		msg := c.env.Messages.RestoredSummaryWith(news, 0, now)
		appendParas(&msg, count, gap)
		msg.Listed = restoredListing(news, ongoing...)
		klog.InfoS("pipeline: restored incidents still failing",
			append(restoredTotals(news), "unchanged", len(ongoing))...)
		// The entry's tier is the loudest news, so a listing that
		// names a page does not read as silent; told ones add nothing.
		c.env.Sink(ctx, incident.Decision{Reason: "restored incidents",
			Incident: incident.Incident{Tier: loudest(news)}}, msg)
		c.rememberListed(news, now)
		return true
	case gap != "":
		c.env.Notify(joinParas(gap, count))
		return true
	}
	c.recordQuietRestart(ctx, now, ongoing)
	return false
}

// recordQuietRestart hands the audit log the restart that told nobody
// anything, naming the ongoing incidents it only counted.
func (c *Collector) recordQuietRestart(
	ctx context.Context, now time.Time, ongoing []incident.Decision,
) {
	if len(ongoing) == 0 {
		return
	}
	msg := c.env.Messages.RestoredSummaryWith(nil, 0, now)
	msg.Carrier = CarrierQuietRestart
	msg.CarrierNote = ongoingCount(len(ongoing), "")
	msg.Listed = restoredListing(nil, ongoing...)
	klog.InfoS("pipeline: quiet restart", "component", "pipeline",
		"unchanged", len(ongoing), "reason", c.env.Restart.Reason)
	c.env.Sink(ctx, incident.Decision{Reason: "restored incidents"}, msg)
}

// ongoingCount says how many ongoing incidents a restart found
// unchanged, such as "12 ongoing issues unchanged; see the dashboard:
// <address>." Empty when there are none.
func ongoingCount(n int, issues string) string {
	if n == 0 {
		return ""
	}
	text := strconv.Itoa(n) + " ongoing issues unchanged"
	if n == 1 {
		text = "1 ongoing issue unchanged"
	}
	if issues != "" {
		return text + "; see the dashboard: " + issues
	}
	return text + "."
}

// appendParas adds each non-empty text, line by line, to the end of a
// composed message: its note, its lines and its rich document.
func appendParas(msg *notification.Message, texts ...string) {
	for _, text := range texts {
		for line := range strings.SplitSeq(text, "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			msg.Note += " " + line
			msg.Lines = append(msg.Lines, line)
			msg.Doc = append(msg.Doc, notification.Block{
				Kind:  notification.Para,
				Spans: []notification.Span{{Text: line}}})
		}
	}
}

// joinParas joins the non-empty texts as paragraphs.
func joinParas(texts ...string) string {
	var parts []string
	for _, text := range texts {
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// rememberListed records that the incidents of listed were named at
// now, keeping the marks of incidents still restored and failing, and
// saves the marker.
func (c *Collector) rememberListed(
	listed []incident.Decision, now time.Time,
) {
	s := &c.Startup.Summary
	mark := c.liveMark(s.Restored, now)
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

// liveMark is a new mark at now keeping, of old, the marks of the
// incidents still restored and failing.
func (c *Collector) liveMark(old *RestoredMark, now time.Time) RestoredMark {
	mark := RestoredMark{At: now, Told: map[string]ToldMark{}}
	if old == nil {
		return mark
	}
	live := map[string]bool{}
	for _, p := range c.env.Incidents.RestoredFailing() {
		live[p.ID] = true
	}
	for _, id := range old.IDs {
		if live[id] {
			mark.IDs = append(mark.IDs, id)
		}
	}
	for id, t := range old.Told {
		if live[id] {
			mark.Told[id] = t
		}
	}
	return mark
}

// saveSummary hands the startup marker to the writer, when there is one.
func (c *Collector) saveSummary() {
	if c.env.SaveStartup != nil {
		c.env.SaveStartup(c.Startup.Summary)
	}
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
