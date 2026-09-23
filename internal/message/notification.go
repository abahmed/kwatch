package message

import (
	"encoding/json"
	"fmt"
	"strings"

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
		if len(report.Diagnosis.NextSteps) > 0 {
			n.Summary.PrimaryAction = commandFor(report, inc)
		}
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
	if d == nil || strings.TrimSpace(d.Cause) == "" {
		return ""
	}
	if deterministicPattern(d.Pattern) ||
		(d.Confidence >= minimumCauseConfidence && len(d.Evidence) > 0) {
		return strings.TrimSuffix(strings.TrimSpace(d.Cause), ".")
	}
	return ""
}

func notificationDetails(report *Report) []NotificationSection {
	var details []NotificationSection
	if report.Identity != nil {
		var lines []string
		if report.Identity.Container != "" {
			lines = append(lines, "Container: "+report.Identity.Container)
		}
		if report.Identity.Image != "" {
			lines = append(lines, "Image: "+report.Identity.Image)
		}
		if report.Identity.Node != "" {
			lines = append(lines, "Node: "+report.Identity.Node)
		}
		if len(lines) > 0 {
			details = append(details, NotificationSection{
				Kind: "identity", Title: "Location", Lines: lines,
			})
		}
	}
	if report.Evidence != nil {
		if report.Evidence.Events != "" {
			details = append(details, NotificationSection{
				Kind: "events", Title: "Events",
				Lines: []string{report.Evidence.Events},
			})
		}
		if report.Evidence.Logs != "" {
			details = append(details, NotificationSection{
				Kind: "logs", Title: "Logs",
				Lines: []string{report.Evidence.Logs},
			})
		}
	}
	if report.OOM != nil {
		var lines []string
		if report.OOM.MemoryLimit != "" {
			lines = append(lines, "Memory limit: "+report.OOM.MemoryLimit)
		}
		if report.OOM.Timeline != "" {
			lines = append(lines, "Memory timeline: "+report.OOM.Timeline)
		}
		if len(lines) > 0 {
			details = append(details, NotificationSection{
				Kind: "memory", Title: "Memory", Lines: lines,
			})
		}
	}
	if report.Probe != nil {
		details = append(details, NotificationSection{
			Kind: "probe", Title: "Probe",
			Lines: []string{report.Probe.ProbeType + " " + report.Probe.Endpoint},
		})
	}
	if report.Pending != nil && len(report.Pending.ResourceRequests) > 0 {
		details = append(details, NotificationSection{
			Kind: "scheduling", Title: "Scheduling",
			Lines: report.Pending.ResourceRequests,
		})
	}
	if report.Runbook != "" {
		details = append(details, NotificationSection{
			Kind: "runbook", Title: "Runbook", Lines: []string{report.Runbook},
		})
	}
	if report.Resolution != nil {
		lines := []string{report.Resolution.Summary}
		if report.Resolution.Evidence != "" {
			lines = append(lines, report.Resolution.Evidence)
		}
		details = append(details, NotificationSection{
			Kind: "recovery", Title: "Recovery", Lines: lines,
		})
	}
	return details
}

func commandFor(report *Report, inc *model.Incident) *Command {
	if report == nil || inc == nil {
		return nil
	}
	ref := inc.Ref()
	if ref.Name == "" && report.Name != "" {
		ref = model.NewObjectRef(
			report.Resource, report.Namespace, report.Name,
		)
	}
	if ref.Name == "" || strings.ContainsAny(ref.Name, " \n\t") {
		args := []string{"kubectl", "get", report.Resource}
		if report.Namespace != "" {
			args = append(args, "-n", report.Namespace)
		}
		return &Command{Label: "List affected resources", Args: args}
	}
	args := []string{"kubectl", "describe", ref.Kind, ref.Name}
	if report.Namespace != "" {
		args = append(args, "-n", report.Namespace)
	}
	return &Command{Label: "Inspect resource", Args: args}
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
