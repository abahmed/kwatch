package message

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// Notification is the provider-neutral semantic notification. Providers may
// choose different layouts, but they must not reconstruct incident meaning
// from raw hints or insight prose.
type Notification struct {
	DeliveryID      string                `json:"deliveryId"`
	ConversationKey string                `json:"conversationKey"`
	Revision        uint64                `json:"revision"`
	Action          model.IncidentAction  `json:"-"`
	Summary         NotificationSummary   `json:"summary"`
	Details         []NotificationSection `json:"details,omitempty"`
	Diagnostic      DiagnosticMetadata    `json:"-"`
}

type NotificationSummary struct {
	Emoji         string   `json:"emoji"`
	Title         string   `json:"title"`
	Severity      string   `json:"severity"`
	Location      Location `json:"location"`
	Story         string   `json:"story,omitempty"`
	Impact        string   `json:"impact,omitempty"`
	Timing        string   `json:"timing,omitempty"`
	Cause         string   `json:"cause,omitempty"`
	PrimaryAction *Command `json:"primaryAction,omitempty"`
}

type Location struct {
	Cluster   string `json:"cluster,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Resource  string `json:"resource,omitempty"`
	Name      string `json:"name,omitempty"`
}

type Command struct {
	Label string   `json:"label"`
	Args  []string `json:"args"`
}

type NotificationSection struct {
	Kind  string   `json:"kind"`
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}

type DiagnosticMetadata struct {
	Pattern       string
	Confidence    float64
	EvidenceCount int
}

// MarshalJSON keeps the wire contract provider-neutral while retaining the
// typed action for in-process renderers. Diagnostic confidence is deliberately
// excluded: it belongs in audit/debug output, never in provider payloads.
func (n Notification) MarshalJSON() ([]byte, error) {
	type payload struct {
		DeliveryID      string                `json:"deliveryId"`
		ConversationKey string                `json:"conversationKey"`
		Revision        uint64                `json:"revision"`
		Action          string                `json:"action"`
		Summary         NotificationSummary   `json:"summary"`
		Details         []NotificationSection `json:"details,omitempty"`
	}
	return json.Marshal(payload{
		DeliveryID:      n.DeliveryID,
		ConversationKey: n.ConversationKey,
		Revision:        n.Revision,
		Action:          n.Action.String(),
		Summary:         n.Summary,
		Details:         n.Details,
	})
}

// NotificationFromReport converts the fully composed report into a stable
// provider-neutral summary and bounded supporting sections.
func NotificationFromReport(
	report *Report,
	inc *model.Incident,
	insPattern string,
	confidence float64,
	evidenceCount int,
) *Notification {
	if report == nil || inc == nil {
		return nil
	}
	n := &Notification{
		DeliveryID:      inc.DeliveryID(parseAction(report.Action)),
		ConversationKey: string(inc.Key),
		Revision:        inc.Revision,
		Action:          parseAction(report.Action),
		Summary: NotificationSummary{
			Emoji:    report.Summary.Emoji,
			Title:    report.Summary.Label,
			Severity: report.Severity,
			Location: Location{
				Cluster:   report.Cluster,
				Namespace: report.Namespace,
				Resource:  report.Resource,
				Name:      report.Name,
			},
			Timing: report.Summary.Duration,
			Story:  notificationStory(report),
		},
		Diagnostic: DiagnosticMetadata{
			Pattern:       insPattern,
			Confidence:    confidence,
			EvidenceCount: evidenceCount,
		},
	}
	if report.Diagnosis != nil {
		n.Summary.Cause = causeText(report.Diagnosis)
		n.Summary.Impact = report.Diagnosis.Impact
	}
	if n.Summary.Impact == "" && len(inc.AffectedMembers) > 0 {
		n.Summary.Impact = pluralCount(
			len(inc.AffectedMembers), "affected resource",
		)
	}
	for _, line := range notificationDetails(report) {
		n.Details = append(n.Details, line)
	}
	return n
}

func causeText(d *DiagnosisSection) string {
	if !causeIsRenderable(d) {
		return ""
	}
	cause := strings.TrimSuffix(strings.TrimSpace(d.Cause), ".")
	if d.CauseState == insight.CauseLikely {
		return "Likely: " + cause
	}
	return cause
}

func notificationStory(report *Report) string {
	if report.Action != "resolved" {
		story := Narrative(report)
		if report.OOM != nil && report.OOM.MemoryLimit != "" {
			story = joinStory(
				story,
				"The container memory limit is "+
					report.OOM.MemoryLimit+".",
			)
		}
		if report.Probe != nil && report.Probe.Endpoint != "" &&
			!strings.Contains(story, report.Probe.Endpoint) {
			story = joinStory(story, fmt.Sprintf(
				"The %s probe to %s is failing.",
				report.Probe.ProbeType, report.Probe.Endpoint,
			))
		}
		if report.Pending != nil && report.Pending.Delay != "" &&
			!strings.Contains(story, report.Pending.Delay) {
			story = joinStory(
				story, "The pod has waited "+report.Pending.Delay+
					" to schedule.",
			)
		}
		if report.Pending != nil &&
			len(report.Pending.ResourceRequests) > 0 {
			story = joinStory(
				story, "Requested resources: "+
					strings.Join(report.Pending.ResourceRequests, "; ")+".",
			)
		}
		return story
	}
	if report.Resolution == nil {
		return ""
	}
	parts := []string{}
	if report.Summary.Duration != "" {
		parts = append(parts, "Recovered after "+report.Summary.Duration+".")
	}
	for _, detail := range []string{
		report.Resolution.Summary,
		report.Resolution.Evidence,
	} {
		if !meaningfulRecoveryDetail(detail) {
			continue
		}
		parts = append(parts, capitalizeSentence(
			strings.TrimSuffix(detail, "."),
		)+".")
	}
	return strings.Join(parts, " ")
}

func joinStory(story, detail string) string {
	if story == "" {
		return detail
	}
	return story + " " + detail
}

func notificationDetails(report *Report) []NotificationSection {
	var details []NotificationSection
	if report.Evidence != nil {
		if report.Evidence.Logs != "" {
			details = append(details, NotificationSection{
				Kind: "logs", Title: "Logs",
				Lines: []string{report.Evidence.Logs},
			})
		}
	}
	if report.Runbook != "" {
		details = append(details, NotificationSection{
			Kind: "runbook", Title: "Runbook", Lines: []string{report.Runbook},
		})
	}
	return details
}

func pluralCount(count int, label string) string {
	if count == 1 {
		return "1 " + label
	}
	return fmt.Sprintf("%d %ss", count, label)
}

func parseAction(action string) model.IncidentAction {
	switch action {
	case "create":
		return model.ActionCreate
	case "update":
		return model.ActionUpdate
	case "resolved":
		return model.ActionResolved
	default:
		return model.ActionSkip
	}
}
