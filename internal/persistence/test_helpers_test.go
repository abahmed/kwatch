package persistence

import (
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
)

func newTestManager(
	client kubernetes.Interface,
	namespace string,
) *Manager {
	return NewManagerWithClock(client, namespace, clock.RealClock{})
}

func newTestIncidentEngine(cfg incident.Config) *incident.Engine {
	return incident.NewEngineWithClock(cfg, clock.RealClock{})
}
