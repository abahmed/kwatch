package message

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/insight"
)

// Narrative is the provider-neutral explanation of an incident. Providers
// may wrap it in native formatting, but must not rebuild its meaning.
const minimumCauseConfidence = 0.75

// causeSentence states the diagnosis in proportion to how much kwatch
// actually knows. It deliberately avoids a fixed "Cause:" label so the
// notification reads as an explanation rather than a populated form.
func causeSentence(d *DiagnosisSection) string {
	if !causeIsRenderable(d) {
		return ""
	}
	cause := strings.TrimSuffix(d.Cause, ".")
	if d.CauseState == insight.CauseLikely {
		cause = "likely, " + cause
	}
	return capitalizeSentence(cause) + "."
}

func causeIsRenderable(d *DiagnosisSection) bool {
	if d == nil || strings.TrimSpace(d.Cause) == "" {
		return false
	}
	return d.CauseState != insight.CauseUnknown &&
		d.Confidence >= minimumCauseConfidence && len(d.Evidence) > 0
}

func Narrative(r *Report) string {
	if r == nil {
		return ""
	}
	var sentences []string
	if r.Diagnosis != nil {
		cause := causeSentence(r.Diagnosis)
		if cause != "" {
			sentences = append(sentences, cause)
		}
		if r.Diagnosis.Impact != "" {
			sentences = append(
				sentences,
				impactSentence(r.Diagnosis.Impact),
			)
		}
		if r.Diagnosis.ReplicaState != "" {
			sentences = append(sentences, r.Diagnosis.ReplicaState+".")
		}
		if r.Diagnosis.Flapping != nil {
			sentences = append(sentences, fmt.Sprintf(
				"🔁 This incident changed state %d times in %s.",
				r.Diagnosis.Flapping.Transitions,
				r.Diagnosis.Flapping.Window,
			))
		}
		if r.Diagnosis.Baseline != "" {
			sentences = append(
				sentences, "📈 "+capitalizeSentence(
					r.Diagnosis.Baseline,
				)+".",
			)
		}
		if r.Diagnosis.Maintenance != "" {
			sentences = append(
				sentences, "🚧 "+capitalizeSentence(
					r.Diagnosis.Maintenance,
				)+".",
			)
		}
		if r.Diagnosis.Provisional && cause == "" &&
			r.Diagnosis.UnknownSummary != "" {
			sentences = append(
				sentences, r.Diagnosis.UnknownSummary+".",
			)
		}
		if r.Diagnosis.LogSignal != nil {
			sentences = append(
				sentences, "🧾 Logs suggest "+
					r.Diagnosis.LogSignal.Summary+".",
			)
		}
	}
	return strings.Join(sentences, " ")
}

func impactSentence(impact string) string {
	impact = strings.TrimSuffix(strings.TrimSpace(impact), ".")
	if impact == "" {
		return ""
	}
	if strings.HasPrefix(impact, "affects ") {
		return "This affects " + strings.TrimPrefix(impact, "affects ") + "."
	}
	return capitalizeSentence(impact) + "."
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
		part := fmt.Sprintf(
			"%s %s %s",
			c.Resource,
			c.Reference,
			strings.ToLower(c.Type),
		)
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
