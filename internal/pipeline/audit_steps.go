package pipeline

import (
	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/redact"
)

// auditSteps records the steps of a message for the audit line: at most
// audit.MaxSteps, each redacted first and then cut, so a secret is never
// cut into a piece the redaction no longer recognises. more counts the
// steps left out.
func auditSteps(steps []notification.Step) (out []audit.StepRecord, more int) {
	for _, s := range steps {
		if len(out) == audit.MaxSteps {
			more++
			continue
		}
		rec := audit.StepRecord{Kind: audit.StepAdvice}
		switch {
		case s.Command != "":
			rec.Command = capStep(redact.Evidence(s.Command))
			rec.Kind = audit.StepReadOnly
			if s.Mutating {
				rec.Kind = audit.StepMutating
			}
		default:
			rec.Text = capStep(redact.Evidence(s.Text))
		}
		out = append(out, rec)
	}
	return out, more
}

// capStep cuts text to audit.MaxStepText characters.
func capStep(text string) string {
	runes := []rune(text)
	if len(runes) <= audit.MaxStepText {
		return text
	}
	return string(runes[:audit.MaxStepText-1]) + "…"
}
