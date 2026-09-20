package integration

import (
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
)

func newTestIncidentEngine(cfg incident.Config) *incident.Engine {
	return incident.NewEngineWithClock(cfg, clock.RealClock{})
}
