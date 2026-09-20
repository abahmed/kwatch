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
	timeSource clock.Clock,
) string {
	return RenderIncidentWithInsight(
		inc, action, nil, renderer, clusterName, timeSource,
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
	timeSource clock.Clock,
) string {
	if action == model.ActionSkip {
		return ""
	}
	report := NewReportBuilderWithClock(
		clusterName, clock.Require(timeSource),
	).Build(inc, action, ins)
	return RenderAction(renderer, report)
}
