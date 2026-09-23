package message

import (
	"fmt"
	"strings"
)

// Narrative is the provider-neutral explanation of an incident. Providers
// may wrap it in native formatting, but must not rebuild its meaning.
const minimumCauseConfidence = 0.75

// causeSentence states the diagnosis in proportion to how much kwatch
// actually knows.
func causeSentence(d *DiagnosisSection) string {
	if !causeIsRenderable(d) {
		return ""
	}
	cause := strings.TrimSuffix(d.Cause, ".")
	return "Cause: " + cause + "."
}

func causeIsRenderable(d *DiagnosisSection) bool {
	if d == nil || strings.TrimSpace(d.Cause) == "" {
		return false
	}
	if deterministicPattern(d.Pattern) {
		return true
	}
	return d.Confidence >= minimumCauseConfidence && len(d.Evidence) > 0
}

func deterministicPattern(pattern string) bool {
	switch pattern {
	case "node_failure", "metrics_api_failure", "service_no_endpoints",
		"webhook_backend_failure", "owner_unhealthy", "rollout_failure":
		return true
	default:
		return false
	}
}

func Narrative(r *Report) string {
	if r == nil {
		return ""
	}
	var sentences []string
	if r.State != nil && r.State.Message != "" {
		sentences = append(sentences, r.State.Message)
	}
	if r.Diagnosis != nil {
		if r.Diagnosis.Cause != "" {
			sentences = append(sentences, causeSentence(r.Diagnosis))
		}
		if r.Diagnosis.Impact != "" {
			sentences = append(sentences, capitalizeSentence(r.Diagnosis.Impact)+".")
		}
		if len(r.Diagnosis.Evidence) > 0 {
			// "This is supported by warning events were observed" is not a
			// sentence; a labelled list is.
			sentences = append(
				sentences,
				"Supporting evidence: "+
					strings.Join(r.Diagnosis.Evidence, "; ")+".",
			)
		}
		if len(r.Diagnosis.NextSteps) > 0 {
			sentences = append(sentences, "Start by "+strings.ToLower(strings.TrimSuffix(r.Diagnosis.NextSteps[0], "."))+".")
		}
	}
	return strings.Join(sentences, " ")
}

// ChangeSummary returns the same compact change explanation used by every
// text provider. It intentionally exposes only the first few fields.
func ChangeSummary(r *Report) string {
	if r == nil || r.Changes == nil || len(r.Changes.Items) == 0 {
		return ""
	}
	const show = 3
	var parts []string
	for i, c := range r.Changes.Items {
		if i == show {
			parts = append(parts, fmt.Sprintf("+%d more", len(r.Changes.Items)-show))
			break
		}
		part := fmt.Sprintf("%s %s %s", c.Resource, c.Reference, strings.ToLower(c.Type))
		if c.Age != "" {
			part += " " + c.Age + " ago"
		}
		if len(c.Fields) > 0 {
			field := c.Fields[0]
			part += ": " + field.Path
			if field.Before != "" && field.After != "" {
				part += " changed from " + clipValue(field.Before) +
					" to " + clipValue(field.After)
			}
			if c.Additional > 0 {
				part += fmt.Sprintf(" (+%d more fields)", c.Additional)
			}
		}
		parts = append(parts, part)
	}
	return "A recent change may be related: " + strings.Join(parts, "; ")
}

// maxChangeValue bounds one field value in a change summary. A replaced
// container image or a resized limit fits easily; a serialized manifest does
// not belong in a chat message at all.
const maxChangeValue = 60

func clipValue(value string) string {
	if len(value) <= maxChangeValue {
		return value
	}
	return value[:maxChangeValue-1] + "…"
}

func capitalizeSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
