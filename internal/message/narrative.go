package message

import (
	"fmt"
	"strings"
)

// Narrative is the provider-neutral explanation of an incident. Providers
// may wrap it in native formatting, but must not rebuild its meaning.
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
			cause := "The strongest signal points to " + strings.TrimSuffix(r.Diagnosis.Cause, ".")
			if r.Diagnosis.Confidence > 0 {
				cause += fmt.Sprintf(" (%.0f%% confidence)", r.Diagnosis.Confidence*100)
			}
			sentences = append(sentences, cause+".")
		}
		if r.Diagnosis.Impact != "" {
			sentences = append(sentences, capitalizeSentence(r.Diagnosis.Impact)+".")
		}
		if len(r.Diagnosis.Evidence) > 0 {
			sentences = append(sentences, "This is supported by "+strings.Join(r.Diagnosis.Evidence, "; ")+".")
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
				part += " changed from " + field.Before + " to " + field.After
			}
			if c.Additional > 0 {
				part += fmt.Sprintf(" (+%d more fields)", c.Additional)
			}
		}
		parts = append(parts, part)
	}
	return "A recent change may be related: " + strings.Join(parts, "; ")
}

func capitalizeSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
