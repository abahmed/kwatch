package alert

import (
	"bytes"
	"strings"
	"text/template"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

func incidentToEvent(inc *model.Incident, action model.IncidentAction) *event.Event {
	return &event.Event{
		Resource:      inc.Resource,
		PodName:       inc.Name,
		ContainerName: inc.ContainerName,
		Namespace:     inc.Namespace,
		NodeName:      inc.NodeName,
		Reason:        inc.Reason,
		Events:        inc.Events,
		Logs:          inc.Logs,
		OwnerKind:     inc.OwnerKind,
		RestartCount:  inc.RestartCount,
		Hint:          inc.Hint,
		Severity:      inc.Severity,
		IncludeEvents: inc.IncludeEvents,
		IncludeLogs:   inc.IncludeLogs,
		Action:        action.String(),
		DedupKey:      inc.ID,
	}
}

// NotifyIncident enqueues an incident for delivery to all providers.
// When Start has been called, delivery is asynchronous via per-provider
// buffered channels (non-blocking; drops oldest on full).
// Before Start, delivery is synchronous (deliverAllSync).
// insight is optional; nil means no structured analysis available.

func (a *AlertManager) buildMessage(inc *model.Incident, action model.IncidentAction, ins *insight.Insight, templates map[string]*template.Template) string {
	rb := message.NewReportBuilder(a.clusterName)
	report := rb.Build(inc, action, ins)
	renderer := message.NewPlainTextRenderer()
	msg := message.RenderAction(renderer, report)

	if t, ok := templates[strings.ToLower(inc.Reason)]; ok {
		var buf bytes.Buffer
		err := t.Execute(&buf, templateData{
			Incident: inc,
			Action:   action.String(),
			Message:  msg,
		})
		if err == nil && buf.Len() > 0 {
			return buf.String()
		}
	}

	return msg
}

// fanOut delivers a job to every registered provider channel (non-blocking).
// Must be called with a.mu held (caller must Lock/Unlock).

type templateData struct {
	Incident *model.Incident
	Action   string
	Message  string
}

func truncateMsg(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	suffix := "\n…(truncated)"
	cut := maxLen - len(suffix)
	if cut <= 0 {
		return suffix
	}
	// back up to a valid rune boundary (FIX-4)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

// providerRenderMargin is reserved when sizing evidence so that provider
// renderers with extra formatting overhead (e.g. Discord code fences) still
// produce output within the provider's message limit.

const providerRenderMargin = 64

// clampIncidentForProvider returns a copy of inc with Logs/Events trimmed so
// that the rendered message fits within maxBytes for ThreadProvider delivery
// (whose renderers post a single message with no chunking). It returns the
// original incident when maxBytes <= 0, when the message already fits, or
// when the overflow cannot be attributed to evidence. fullLen is the length
// of the pre-rendered message and avoids a redundant render on the fast path.

func (a *AlertManager) clampIncidentForProvider(inc *model.Incident, action model.IncidentAction, ins *insight.Insight, maxBytes int, tpl map[string]*template.Template, fullLen int) *model.Incident {
	if maxBytes <= 0 || fullLen <= maxBytes || (inc.Logs == "" && inc.Events == "") {
		return inc
	}

	// Render without evidence to learn the exact fixed-size overhead.
	fixed := inc.Clone()
	fixed.Logs = ""
	fixed.Events = ""
	fixedLen := len(a.buildMessage(fixed, action, ins, tpl))
	budget := maxBytes - fixedLen - providerRenderMargin
	if budget <= 0 {
		return inc
	}

	clamped := inc.Clone()
	trimEvidence(clamped, budget)
	return clamped
}

// trimEvidence truncates inc.Logs and inc.Events (with a marker suffix) so
// their rendered sections fit within budget bytes.

func trimEvidence(inc *model.Incident, budget int) {
	if budget <= 0 {
		inc.Logs = ""
		inc.Events = ""
		return
	}
	logs := inc.Logs
	events := inc.Events
	const logHeader = "Logs:\n"
	const evHeader = "Events:\n"
	switch {
	case logs == "":
		inc.Events = truncateMsg(events, budget-len(evHeader))
	case events == "":
		inc.Logs = truncateMsg(logs, budget-len(logHeader))
	default:
		total := len(logs) + len(events)
		if total <= 0 {
			return
		}
		logBudget := int(int64(budget) * int64(len(logs)) / int64(total))
		evBudget := budget - logBudget
		inc.Logs = truncateMsg(logs, logBudget-len(logHeader))
		inc.Events = truncateMsg(events, evBudget-len(evHeader))
	}
}

func defaultMaxBytes(providerName string) int {
	switch strings.ToLower(providerName) {
	case "telegram":
		return 4096
	case "teams":
		return 28000
	case "slack":
		return 40000
	case "discord":
		return 2000
	default:
		return 0 // unlimited
	}
}
