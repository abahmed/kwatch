package app

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controlplane"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/monitor"
)

func configureControlPlaneMonitor(
	runtime config.RuntimeConfig,
	clientset kubernetes.Interface,
	restClient rest.Interface,
	resolver controlplane.HostResolver,
	healthServer *health.HealthServer,
	incidentSink monitor.ObservationSink,
	now func() time.Time,
) (func(context.Context) error, *controlplane.Monitor) {
	if !runtime.ControlPlaneMonitor().Enabled {
		return nil, nil
	}
	monitor := controlplane.NewWithRESTDependencies(
		restClient, clientset, runtime.ControlPlaneMonitor(), incidentSink,
		resolver, clock.Func(now),
	)
	healthServer.SetControlPlaneLister(monitor)
	return monitor.Start, monitor
}
