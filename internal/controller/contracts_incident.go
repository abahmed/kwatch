package controller

import (
	"github.com/abahmed/kwatch/internal/incident"
)

// IncidentSourceConfig supplies the informer-backed sources used for
// attribution. It is intentionally separate from monitor family contracts.
type IncidentSourceConfig interface {
	ConfigureAttributionSources(incident.AttributionSources) error
}

// BaselineSink receives startup state and node inhibition state from the
// controller without exposing the incident engine implementation.
type BaselineSink interface {
	SetBaseline(map[string]map[string]int64)
	SetActiveNodeIncidents([]string)
}
