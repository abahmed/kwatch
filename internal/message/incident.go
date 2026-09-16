package message

import (
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// RenderIncident renders an incident using the supplied renderer. Skip
// actions intentionally produce no message.
func RenderIncident(
	inc *model.Incident,
	action model.IncidentAction,
	renderer Renderer,
	clusterName string,
) string {
	return RenderIncidentWithInsight(
		inc, action, nil, renderer, clusterName,
	)
}

// RenderIncidentWithInsight renders the common report model used by rich
// providers, including optional diagnosis details.
func RenderIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
	renderer Renderer,
	clusterName string,
) string {
	if action == model.ActionSkip {
		return ""
	}
	report := NewReportBuilderWithClock(
		clusterName, clock.RealClock{},
	).Build(inc, action, ins)
	return RenderAction(renderer, report)
}
