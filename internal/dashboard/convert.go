package dashboard

import (
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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
		Untold:          p.Untold(),
		ResolvedAt:      p.Resolved,
		RootCause:       shortCause(red(causeText(p))),
		LeftoverObjects: p.LeftoverObjects(),
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

// buildDetail converts p, adds the context c and fits the result to
// the response limit.
func buildDetail(p incident.Incident, c detailContext) IncidentDetail {
	steps := toSteps(p.Steps)
	item := toListItem(p)
	item.RootCause = redact.Evidence(causeText(p))
	d := IncidentDetail{
		IncidentListItem: item,
		FullTitle:        redact.Evidence(p.FullTitle()),
		FixSteps:         steps,
		Timeline:         toTimeline(p.Timeline),
		CapturedLogLines: redactAll(p.Facts),
		DocsLink:         p.DocsLink,
		Evidence:         toEvidence(p.Evidence),
		Correlation:      toCorrelation(p.Correlation),
		Leftovers:        toLeftovers(p),
	}
	if cut := len(p.Timeline) - len(d.Timeline); cut > 0 {
		d.Truncated = &DetailTruncation{Timeline: cut}
	}
	c.addContext(&d, p)
	d.Summary = summaryOf(&d, p)
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

// titleFor is the incident's problem title, the composer's lead of
// what fails (incident.Incident.Title), else, as a last resort for a
// record that has none, the root in words: "Service api in shop".
func titleFor(p incident.Incident, red func(string) string) string {
	switch title := p.Title(); {
	case title != "":
		return red(title)
	case p.Root.Name != "":
		return rootWords(p.Root)
	default:
		return p.ID
	}
}

// rootWords names an object as a reader says it: its Kind as
// Kubernetes spells it, its name and, when it has one, its namespace
// ("Deployment api in shop", "MutatingWebhookConfiguration hook").
func rootWords(id inventory.EntityID) string {
	out := kindDisplay(id) + " " + id.Name
	if id.Namespace != "" {
		out += " in " + id.Namespace
	}
	return out
}

// kindSpellings maps an entity's qualified kind to the Kind as the API
// spells it, from the coverage catalog.
var kindSpellings = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	for _, e := range kube.CoverageCatalog() {
		id := inventory.EntityID{Group: kube.GroupFor(e.Group),
			Kind: kube.KindFor(e.Kind)}
		out[id.QualifiedKind()] = e.Kind
	}
	return out
})

// kindDisplay is the Kind as Kubernetes spells it, else the kind with
// a capital first letter ("Cluster-dns").
func kindDisplay(id inventory.EntityID) string {
	if k, ok := kindSpellings()[id.QualifiedKind()]; ok {
		return k
	}
	k := string(id.Kind)
	if k == "" {
		return "Object"
	}
	return strings.ToUpper(k[:1]) + k[1:]
}

// lastSeen is the latest sign of life of p: its resolve, else its
// announcement or opening, and any later timeline entry (a failure
// that went on, a member that joined or recovered, a revised cause).
func lastSeen(p incident.Incident) time.Time {
	last := p.Opened
	if !p.Resolved.IsZero() {
		last = p.Resolved
	} else if !p.Announced.IsZero() {
		last = p.Announced
	}
	for _, e := range p.Timeline {
		if e.At.After(last) {
			last = e.At
		}
	}
	return last
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
