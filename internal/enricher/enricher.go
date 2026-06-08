package enricher

import (
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

type Enricher interface {
	Enrich(ev *event.Event, inc *model.Incident)
}

type DefaultEnricher struct{}

func (e *DefaultEnricher) Enrich(ev *event.Event, inc *model.Incident) {
	inc.OwnerKind = ev.OwnerKind
	inc.ContainerName = ev.ContainerName
	inc.RestartCount = ev.RestartCount
	inc.Hint = hintForReason(ev.Reason)
}
