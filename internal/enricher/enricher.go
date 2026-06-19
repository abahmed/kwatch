package enricher

import (
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

var defaultSeverityByOwnerKind = map[string]string{
	"StatefulSet": "high",
}

var defaultSeverityByReason = map[string]string{
	"Evicted":          "medium",
	"ImagePullBackOff": "medium",
}

type Enricher interface {
	Enrich(ev *event.Event, inc *model.Incident)
}

type DefaultEnricher struct {
	SeverityByOwnerKind map[string]string
	SeverityByReason    map[string]string
}

func (e *DefaultEnricher) SetSeverityMap(m map[string]string) {
	e.SeverityByOwnerKind = m
}

func (e *DefaultEnricher) Enrich(ev *event.Event, inc *model.Incident) {
	inc.OwnerKind = ev.OwnerKind
	inc.ContainerName = ev.ContainerName
	if ev.Hint != "" {
		inc.Hint = ev.Hint
	} else {
		inc.Hint = hintForReason(ev.Reason)
	}
	// CD-3: signature-based hints for common patterns
	if sh := signatureHint(ev.Logs); sh != "" {
		inc.Hint = combineHints(inc.Hint, sh)
	}
	inc.Logs = ev.Logs
	inc.Events = ev.Events
	inc.IncludeEvents = ev.IncludeEvents
	inc.IncludeLogs = ev.IncludeLogs
	newSev := ev.Severity
	if newSev == "" {
		newSev = e.resolveSeverity(ev.OwnerKind, ev.Reason)
	}
	// severity is strictly monotonic (sticky escalation): once raised, it never
	// downgrades until the incident resolves. This is intentional — a runtime
	// config change via CRD watcher that lowers SeverityByReason or
	// SeverityByOwnerKind will not take effect on already-open incidents.
	if severityRank(newSev) >= severityRank(inc.Severity) {
		inc.Severity = newSev
	}
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "high":
		return 2
	case "medium":
		return 1
	default:
		return 0
	}
}

func (e *DefaultEnricher) resolveSeverity(ownerKind, reason string) string {
	if e.SeverityByReason != nil {
		if s, ok := e.SeverityByReason[reason]; ok {
			return s
		}
	}
	if s, ok := defaultSeverityByReason[reason]; ok {
		return s
	}
	if e.SeverityByOwnerKind != nil {
		if s, ok := e.SeverityByOwnerKind[ownerKind]; ok {
			return s
		}
		if s, ok := e.SeverityByOwnerKind["default"]; ok {
			return s
		}
	}
	if s, ok := defaultSeverityByOwnerKind[ownerKind]; ok {
		return s
	}
	return "normal"
}
