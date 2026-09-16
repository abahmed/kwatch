package pvc

import (
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/persistence"
)

func newTestPersistenceManager(
	client kubernetes.Interface,
	namespace string,
) *persistence.Manager {
	return persistence.NewManagerWithClock(
		client, namespace, clock.RealClock{},
	)
}

func newTestPvcMonitorWithState(
	client kubernetes.Interface,
	monitorConfig *config.PvcMonitor,
	incidentSink monitor.ObservationSink,
	stateStore StateStore,
) *PvcMonitor {
	var normalized config.PvcMonitor
	if monitorConfig != nil {
		normalized = *monitorConfig
	}
	return newPvcMonitor(
		client, normalized, incidentSink, stateStore,
		clock.RealClock{}.Now,
	)
}
