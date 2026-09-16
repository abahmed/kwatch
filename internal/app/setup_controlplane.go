package app

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controlplane"
	"github.com/abahmed/kwatch/internal/monitor"
)

func configureControlPlaneMonitor(
	runtime config.RuntimeConfig,
	clientset kubernetes.Interface,
	restClient rest.Interface,
	resolver controlplane.HostResolver,
	incidentSink monitor.ObservationSink,
	now func() time.Time,
) (func(context.Context) error, *controlplane.Monitor) {
	if !runtime.Monitors().ControlPlane().Enabled {
		return nil, nil
	}
	monitor := controlplane.NewWithRESTDependencies(
		restClient, clientset, runtime.Monitors().ControlPlane(), incidentSink,
		resolver, clock.Func(now),
	)
	return monitor.Start, monitor
}
