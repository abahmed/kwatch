package app

import (
	"strings"

	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/redact"
)

// runbookStepPrefix starts the step compose writes for a configured
// runbook: "Runbook for <reason>: <url>".
const runbookStepPrefix = "Runbook for "

// dashboardDetail is what a sent decision leaves on its incident for the
// dashboard detail page: the composer's title, the runbook link and the
// captured output and evidence, all already redacted.
func dashboardDetail(
	d incident.Decision, m notification.Message,
) incident.Detail {
	out := incident.Detail{Headline: m.Title, DocsLink: runbookLink(m)}
	out.Facts = append(out.Facts, d.Facts.Output...)
	for _, f := range d.Facts.Evidence {
		out.Facts = append(out.Facts, f.Text)
	}
	out.Steps = fixSteps(m)
	return out
}

// fixSteps are the message's steps without the runbook step, which the
// dashboard shows as its link, redacted like everything it serves.
func fixSteps(m notification.Message) []incident.FixStep {
	var out []incident.FixStep
	for _, s := range m.Steps {
		if strings.HasPrefix(s.Text, runbookStepPrefix) && s.Command == "" {
			continue
		}
		out = append(out, incident.FixStep{
			Text:    redact.Evidence(s.Text),
			Command: redact.Evidence(s.Command),
		})
	}
	return out
}

// runbookLink is the first runbook link among the message's steps.
func runbookLink(m notification.Message) string {
	for _, s := range m.Steps {
		rest, ok := strings.CutPrefix(s.Text, runbookStepPrefix)
		if !ok {
			continue
		}
		if _, url, ok := strings.Cut(rest, ": "); ok {
			return url
		}
	}
	return ""
}

// dashboardHealth reports each provider's delivery health to the
// dashboard.
func dashboardHealth(
	m *delivery.Manager,
) func() []dashboard.ChannelHealth {
	return func() []dashboard.ChannelHealth {
		out := make([]dashboard.ChannelHealth, 0)
		for _, h := range m.ProviderHealth() {
			out = append(out, dashboard.ChannelHealth{
				Channel:          h.Name,
				Type:             h.Name,
				LastSuccess:      h.LastSuccess,
				LastError:        h.LastError,
				LastErrorMessage: h.LastErrorMessage,
			})
		}
		return out
	}
}
