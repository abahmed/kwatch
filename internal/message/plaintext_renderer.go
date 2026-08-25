package message

import (
	"fmt"
	"strings"
)

// PlainTextRenderer produces plain text from a Report, suitable for
// Email, Telegram, Webhook, and other text-only providers.
type PlainTextRenderer struct{}

// NewPlainTextRenderer returns a PlainTextRenderer.
func NewPlainTextRenderer() *PlainTextRenderer {
	return &PlainTextRenderer{}
}

func (pr *PlainTextRenderer) RenderCreate(r *Report) string {
	var s []string

	s = append(s, pr.renderHeader(r))
	s = append(s, pr.renderDetails(r))

	if r.Diagnosis != nil && r.Diagnosis.Hint != "" {
		s = append(s, "Hint: "+r.Diagnosis.Hint)
	}
	if r.Diagnosis != nil && r.Diagnosis.Cause != "" {
		s = append(s, "Likely cause: "+r.Diagnosis.Cause)
	}
	if r.Diagnosis != nil && r.Diagnosis.Impact != "" {
		s = append(s, "Impact: "+r.Diagnosis.Impact)
	}
	if r.Diagnosis != nil && r.Diagnosis.Pattern != "" {
		s = append(s, "Pattern: "+r.Diagnosis.Pattern)
	}

	s = append(s, pr.renderTypeSpecific(r))
	s = append(s, pr.renderSuppressed(r))
	s = append(s, pr.renderEvidence(r))

	if r.Changes != nil && len(r.Changes.Items) > 0 {
		s = append(s, pr.renderChanges(r.Changes))
	}

	if r.Runbook != "" {
		s = append(s, fmt.Sprintf("Runbook: %s", r.Runbook))
	}

	return strings.Join(s, "\n")
}

func (pr *PlainTextRenderer) RenderUpdate(r *Report) string {
	var s []string

	s = append(s, pr.renderHeader(r))
	s = append(s, pr.renderDetails(r))

	if r.Diagnosis != nil && r.Diagnosis.Hint != "" {
		s = append(s, "Hint: "+r.Diagnosis.Hint)
	}
	if r.Diagnosis != nil && r.Diagnosis.Cause != "" {
		s = append(s, "Likely cause: "+r.Diagnosis.Cause)
	}

	s = append(s, pr.renderTypeSpecific(r))
	s = append(s, pr.renderEvidence(r))

	return strings.Join(s, "\n")
}

func (pr *PlainTextRenderer) RenderResolved(r *Report) string {
	var s []string

	header := fmt.Sprintf("%s Resolved — %s", r.Summary.Emoji, r.Reason)
	if r.Summary.Label != "" && r.Summary.Label != r.Reason {
		header += fmt.Sprintf(" — %s", r.Summary.Label)
	}
	if r.Resource != "" && r.Name != "" {
		header += fmt.Sprintf(" in %s/%s", r.Resource, r.Name)
	} else if r.Name != "" {
		header += " — " + r.Name
	}
	if r.Namespace != "" {
		header += fmt.Sprintf(" (%s)", r.Namespace)
	}
	s = append(s, header)

	var info []string
	info = append(info, fmt.Sprintf("Duration: %s", r.Summary.Duration))
	if r.Identity != nil && r.Identity.Node != "" {
		info = append(info, fmt.Sprintf("Node: %s", r.Identity.Node))
	}
	if r.State != nil && r.State.ExitCode > 0 {
		info = append(info, fmt.Sprintf("Exit Code: %d", r.State.ExitCode))
	}
	info = append(info, fmt.Sprintf("Total events: %d", r.Summary.Count))
	if r.Summary.Peak > 0 {
		info = append(info, fmt.Sprintf("Peak pods: %d", r.Summary.Peak))
	}
	s = append(s, strings.Join(info, " · "))

	return strings.Join(s, "\n")
}

// --- Internal rendering helpers ---

func (pr *PlainTextRenderer) renderHeader(r *Report) string {
	header := fmt.Sprintf("%s %s", r.Summary.Emoji, r.Reason)
	if r.Summary.Label != "" && r.Summary.Label != r.Reason {
		header += fmt.Sprintf(" — %s", r.Summary.Label)
	}
	if r.Resource != "" && r.Name != "" {
		header += fmt.Sprintf(" in %s/%s", r.Resource, r.Name)
	} else if r.Name != "" {
		header += " — " + r.Name
	}
	if r.Namespace != "" {
		header += fmt.Sprintf(" (%s)", r.Namespace)
	}
	if r.Severity != "" && r.Severity != "normal" {
		header += " — " + r.Severity
	}
	return header
}

func (pr *PlainTextRenderer) renderDetails(r *Report) string {
	var parts []string
	if r.Identity != nil {
		if r.Identity.Container != "" {
			parts = append(parts, fmt.Sprintf("Container: %s", r.Identity.Container))
		}
		if r.Identity.Image != "" {
			parts = append(parts, fmt.Sprintf("Image: %s", r.Identity.Image))
		}
		if r.Identity.Node != "" {
			parts = append(parts, fmt.Sprintf("Node: %s", r.Identity.Node))
		}
		if r.Identity.OwnerKind != "" {
			parts = append(parts, fmt.Sprintf("Kind: %s", r.Identity.OwnerKind))
		}
	}
	if r.State != nil {
		if r.State.Message != "" {
			parts = append(parts, fmt.Sprintf("Message: %s", r.State.Message))
		}
		if r.State.ExitCode > 0 {
			parts = append(parts, fmt.Sprintf("Exit Code: %d", r.State.ExitCode))
		}
		if r.State.Restarts > 0 {
			parts = append(parts, fmt.Sprintf("Restarts: %d", r.State.Restarts))
		}
		parts = append(parts, fmt.Sprintf("Count: %d", r.Summary.Count))
		parts = append(parts, fmt.Sprintf("Duration: %s", r.State.Duration))
	}
	if r.Summary.Peak > 0 {
		parts = append(parts, fmt.Sprintf("Peak pods: %d", r.Summary.Peak))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

func (pr *PlainTextRenderer) renderTypeSpecific(r *Report) string {
	var parts []string

	if r.OOM != nil {
		if r.OOM.MemoryLimit != "" {
			parts = append(parts, fmt.Sprintf("Memory limit: %s", r.OOM.MemoryLimit))
		}
		if r.OOM.IsLeak {
			parts = append(parts, fmt.Sprintf("Potential memory leak (%d OOMs in %dm)",
				r.OOM.LeakCount, r.OOM.WindowMin))
		}
		if r.OOM.Timeline != "" {
			parts = append(parts, fmt.Sprintf("Memory before crash: %s", r.OOM.Timeline))
		}
	}

	if r.Probe != nil {
		parts = append(parts, fmt.Sprintf("Probe: %s %s", r.Probe.ProbeType, r.Probe.Endpoint))
	}

	if r.Image != nil && r.Image.RegistryHint != "" {
		parts = append(parts, r.Image.RegistryHint)
	}

	if r.Pending != nil {
		if r.Pending.Delay != "" {
			parts = append(parts, fmt.Sprintf("Scheduling delay: %s", r.Pending.Delay))
		}
		for _, req := range r.Pending.ResourceRequests {
			parts = append(parts, req)
		}
	}

	return strings.Join(parts, "\n")
}

func (pr *PlainTextRenderer) renderSuppressed(r *Report) string {
	if r.SuppressedPods == 0 {
		return ""
	}
	s := fmt.Sprintf("%d other pod(s) on this node also crashed (grouped to reduce noise)", r.SuppressedPods)
	if len(r.SuppressedPodSummaries) > 0 && len(r.SuppressedPodSummaries) <= 5 {
		var pods []string
		for _, ps := range r.SuppressedPodSummaries {
			pods = append(pods, fmt.Sprintf("  %s/%s (%s)", ps.Namespace, ps.PodName, ps.Reason))
		}
		s += "\n" + strings.Join(pods, "\n")
	}
	return s
}

func (pr *PlainTextRenderer) renderEvidence(r *Report) string {
	if r.Evidence == nil {
		return ""
	}
	var parts []string
	if r.Evidence.Logs != "" {
		parts = append(parts, "Logs:\n"+r.Evidence.Logs)
	}
	if r.Evidence.Events != "" {
		parts = append(parts, "Events:\n"+r.Evidence.Events)
	}
	return strings.Join(parts, "\n")
}

func (pr *PlainTextRenderer) renderChanges(c *ChangesSection) string {
	var parts []string
	parts = append(parts, "Recent changes:")
	for _, item := range c.Items {
		parts = append(parts, fmt.Sprintf("  %s %s %s", item.Resource, item.Reference, item.Type))
	}
	if c.AffectedCount > 0 {
		parts = append(parts, fmt.Sprintf("Affected resources: %d", c.AffectedCount))
	}
	return strings.Join(parts, "\n")
}
