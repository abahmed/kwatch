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
	return IncidentListItem{
		ID:           p.ID,
		Title:        titleFor(p),
		Severity:     p.Tier.String(),
		State:        p.State.String(),
		Namespace:    p.Root.Namespace,
		Workload:     p.Root.Name,
		WorkloadKind: string(p.Root.Kind),
		FirstSeen:    p.Opened,
		LastSeen:     lastSeen(p),
		RepeatCount:  p.RepeatCount,
		AckedBy:      ackedBy(p),
		ResolvedAt:   p.Resolved,
	}
}

func toDetail(p incident.Incident) IncidentDetail {
	steps, commands, text := toSteps(p.Steps)
	return IncidentDetail{
		IncidentListItem: toListItem(p),
		RootCause:        redact.Evidence(causeText(p)),
		SuggestedFix:     text,
		FixSteps:         steps,
		FixCommands:      commands,
		Timeline:         toTimeline(p.Timeline),
		CapturedLogLines: redactAll(p.Facts),
		DocsLink:         p.DocsLink,
	}
}

// toSteps returns the steps, their commands alone and their sentences
// joined, all redacted again.
func toSteps(in []incident.FixStep) ([]FixStep, []string, string) {
	steps := make([]FixStep, 0, len(in))
	commands := make([]string, 0, len(in))
	var text []string
	for _, s := range in {
		step := FixStep{Text: redact.Evidence(s.Text),
			Command: redact.Evidence(s.Command)}
		steps = append(steps, step)
		if step.Command != "" {
			commands = append(commands, step.Command)
		}
		if step.Text != "" {
			text = append(text, step.Text)
		}
	}
	return steps, commands, strings.Join(text, "\n")
}

// titleFor is the composer's headline of the last message, else the
// root's name.
func titleFor(p incident.Incident) string {
	switch {
	case p.Headline != "":
		return redact.Evidence(p.Headline)
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
			At:     e.At,
			Kind:   kind,
			Reason: reason,
			Text:   redact.Evidence(e.Text),
			Entity: entityIDString(e.Entity),
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
