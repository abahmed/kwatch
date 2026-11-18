package pipeline

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/redact"
)

// RunbookStepPrefix starts the step compose writes for a configured
// runbook: "Runbook for <reason>: <url>".
const RunbookStepPrefix = "Runbook for "

// FixSteps are the message's steps without the runbook step, which the
// dashboard shows as its link, redacted like everything it serves.
func FixSteps(m notification.Message) []incident.FixStep {
	var out []incident.FixStep
	for _, s := range m.Steps {
		if strings.HasPrefix(s.Text, RunbookStepPrefix) && s.Command == "" {
			continue
		}
		out = append(out, incident.FixStep{
			Text:    redact.Evidence(s.Text),
			Command: redact.Evidence(s.Command),
		})
	}
	return out
}
