package dashboard

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxTimeline bounds the timeline a detail response carries: the most
// recent entries.
const maxTimeline = 100

func toListItem(p incident.Incident) IncidentListItem {
	return toListItemWith(p, redact.Evidence)
}

// toListItemWith is toListItem with a given redaction, so a list can
// reuse what it redacted on the last read.
func toListItemWith(
	p incident.Incident, red func(string) string,
) IncidentListItem {
	return IncidentListItem{
		ID:              p.ID,
		Title:           titleFor(p, red),
		Severity:        p.Tier.String(),
		State:           p.State.String(),
		Namespace:       p.Root.Namespace,
		Workload:        p.Root.Name,
		WorkloadKind:    string(p.Root.Kind),
		FirstSeen:       p.Opened,
		LastSeen:        lastSeen(p),
		RepeatCount:     p.RepeatCount,
		AckedBy:         ackedBy(p),
		Acked:           p.Ack != nil,
		StoppedTracking: stoppedTracking(p),
		ResolvedAt:      p.Resolved,
		RootCause:       shortCause(red(causeText(p))),
	}
}

// maxListCause bounds the cause a list item carries: one line on a row,
// and a long cause must not push incidents out of the list's response
// budget. The detail carries it whole.
const maxListCause = 200

func shortCause(cause string) string {
	runes := []rune(cause)
	if len(runes) <= maxListCause {
		return cause
	}
	return strings.TrimSpace(string(runes[:maxListCause-1])) + "…"
}

// toDetail is the detail of p alone, without the server's context.
func toDetail(p incident.Incident) IncidentDetail {
	return buildDetail(p, detailContext{})
}

// buildDetail converts p, adds the context c and fits the result to
// the response limit.
func buildDetail(p incident.Incident, c detailContext) IncidentDetail {
	steps := toSteps(p.Steps)
	item := toListItem(p)
	item.RootCause = redact.Evidence(causeText(p))
	d := IncidentDetail{
		IncidentListItem: item,
		FixSteps:         steps,
		Timeline:         toTimeline(p.Timeline),
		CapturedLogLines: redactAll(p.Facts),
		DocsLink:         p.DocsLink,
		Evidence:         toEvidence(p.Evidence),
		Correlation:      toCorrelation(p.Correlation),
	}
	if cut := len(p.Timeline) - len(d.Timeline); cut > 0 {
		d.Truncated = &DetailTruncation{Timeline: cut}
	}
	c.addContext(&d, p)
	fitDetail(&d, detailBudget)
	return d
}

// toSteps returns the steps, redacted again.
func toSteps(in []incident.FixStep) []FixStep {
	steps := make([]FixStep, 0, len(in))
	for _, s := range in {
		steps = append(steps, FixStep{Text: redact.Evidence(s.Text),
			Command: redact.Evidence(s.Command)})
	}
	return steps
}

// titleFor is the composer's headline of the last message, else the
// root's name.
func titleFor(p incident.Incident, red func(string) string) string {
	switch {
	case p.Headline != "":
		return red(p.Headline)
	case p.Root.Name != "":
		return p.Root.String()
	default:
		return p.ID
	}
}

func lastSeen(p incident.Incident) time.Time {
	if !p.Resolved.IsZero() {
		return p.Resolved
	}
	if !p.Announced.IsZero() {
		return p.Announced
	}
	return p.Opened
}

func ackedBy(p incident.Incident) string {
	if p.Ack != nil {
		return p.Ack.On.String()
	}
	return ""
}

func causeText(p incident.Incident) string {
	if p.Cause != nil {
		return p.Cause.Summary
	}
	return ""
}

// redactAll runs every line through redact again before it is served:
// lines are redacted when captured, this keeps the API safe on its own.
func redactAll(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, redact.Evidence(l))
	}
	return out
}

func toTimeline(events []incident.Event) []Event {
	if len(events) > maxTimeline {
		events = events[len(events)-maxTimeline:]
	}
	out := make([]Event, 0, len(events))
	for _, e := range events {
		kind, reason := classifyEvent(e.Text)
		out = append(out, Event{
			At:      e.At,
			Kind:    kind,
			Reason:  reason,
			Text:    redact.Evidence(e.Text),
			Entity:  entityIDString(e.Entity),
			Repeats: e.Repeats,
		})
	}
	return out
}

func entityIDString(e *inventory.EntityID) string {
	if e == nil {
		return ""
	}
	return e.String()
}
