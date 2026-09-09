package message

import (
	"fmt"
	"strings"
)

// Narrative is the provider-neutral explanation of an incident. Providers
// may wrap it in native formatting, but must not rebuild its meaning.
// weakCause is the confidence below which a diagnosis is a suggestion rather
// than a finding.
//
// The insight engine will name a cause from graph topology alone -- this
// workload depends on that ConfigMap, the ConfigMap changed recently -- and
// then halve its own confidence for having no supporting evidence. The result
// was a 26% guess introduced with the same words as a 90% certainty:
// "The strongest signal points to ...". Reading the number is not the reader's
// job; the sentence should carry its own weight.
const weakCause = 0.35

// causeSentence states the diagnosis in proportion to how much kwatch
// actually knows.
func causeSentence(d *DiagnosisSection) string {
	cause := strings.TrimSuffix(d.Cause, ".")
	lead := "The strongest signal points to "
	if d.Confidence > 0 && d.Confidence < weakCause {
		lead = "On weak evidence, one possibility is "
	}
	if d.Confidence > 0 {
		return fmt.Sprintf(
			"%s%s (%.0f%% confidence).",
			lead,
			cause,
			d.Confidence*100,
		)
	}
	return lead + cause + "."
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
