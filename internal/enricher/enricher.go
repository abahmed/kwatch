package enricher

import (
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

var defaultSeverityByOwnerKind = map[string]string{
	"StatefulSet": "high",
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
	inc.Logs = ev.Logs
	inc.Events = ev.Events
	inc.IncludeEvents = ev.IncludeEvents
	inc.IncludeLogs = ev.IncludeLogs
	if ev.Severity != "" {
		inc.Severity = ev.Severity
	} else {
		inc.Severity = e.resolveSeverity(ev.OwnerKind, ev.Reason)
	}
}

func (e *DefaultEnricher) resolveSeverity(ownerKind, reason string) string {
	if e.SeverityByReason != nil {
		if s, ok := e.SeverityByReason[reason]; ok {
			return s
		}
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
