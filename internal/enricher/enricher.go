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
}

func (e *DefaultEnricher) Enrich(ev *event.Event, inc *model.Incident) {
	inc.OwnerKind = ev.OwnerKind
	inc.ContainerName = ev.ContainerName
	inc.RestartCount = ev.RestartCount
	if ev.Hint != "" {
		inc.Hint = ev.Hint
	} else {
		inc.Hint = hintForReason(ev.Reason)
	}
	inc.Logs = ev.Logs
	inc.Events = ev.Events
	inc.Severity = e.resolveSeverity(ev.OwnerKind)
}

func (e *DefaultEnricher) resolveSeverity(ownerKind string) string {
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
