package util

import (
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

// OrDefault returns s if non-empty, otherwise returns def.
func OrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Chunks splits s into slices of at most chunkSize characters.
func Chunks(s string, chunkSize int) []string {
	if chunkSize >= len(s) {
		return []string{s}
	}

	chunks := make([]string, 0, (len(s)-1)/chunkSize+1)
	currentLen := 0
	currentStart := 0

	for i := range s {
		if currentLen == chunkSize {
			chunks = append(chunks, s[currentStart:i])
			currentLen = 0
			currentStart = i
		}
		currentLen++
	}

	chunks = append(chunks, s[currentStart:])
	return chunks
}

// RenderIncident renders an incident using the Report model and the given
// renderer, returning the formatted text. Returns empty string for skip
// actions.
func RenderIncident(
	inc *model.Incident,
	action model.IncidentAction,
	renderer message.Renderer,
	clusterName string,
) string {
	return RenderIncidentWithInsight(inc, action, nil, renderer, clusterName)
}

// RenderIncidentWithInsight is RenderIncident with the insight engine's
// diagnosis included. Every provider that renders through the Report model
// gets cause, impact and recent changes from this one place — before it, ten
// providers hard-coded nil here and silently dropped the diagnosis.
func RenderIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
	renderer message.Renderer,
	clusterName string,
) string {
	if action == model.ActionSkip {
		return ""
	}
	report := message.NewReportBuilder(clusterName).Build(inc, action, ins)
	return message.RenderAction(renderer, report)
}
