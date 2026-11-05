package app

import (
	"os"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// baseIncidentConfig is the lifecycle configuration every edition
// shares: which kinds and objects kwatch can verify, and kwatch's own
// namespace and Deployment (the chart's POD_NAMESPACE and POD_NAME), so
// its own volume wait at start stays quiet.
func (p *pipelineBuild) baseIncidentConfig() incident.Config {
	return incident.Config{
		Verifiable: func(kind inventory.Kind) bool {
			return p.source == nil || p.source.Verifiable(kind)
		},
		VerifiableObject: func(id inventory.EntityID) bool {
			return p.source == nil || p.source.VerifiableObject(id)
		},
		OwnNamespace: envOrKwatch("POD_NAMESPACE"),
		OwnWorkload:  incident.OwnWorkloadOf(os.Getenv("POD_NAME")),
	}
}
