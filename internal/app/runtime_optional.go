package app

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/kubeletmetrics"
)

type optionalRuns struct {
	tlsSweep        func()
	statusRun       func(context.Context)
	metricsRun      func(context.Context)
	probeRun        func(context.Context)
	kubeletRun      func(context.Context)
	storageRun      func(context.Context)
	networkRun      func(context.Context)
	securityRun     func(context.Context)
	controlPlaneRun func(context.Context)
}

func configureOptionalRuns(
	runtime config.RuntimeConfig,
	boot *bootstrap,
	ctl *controller.Controller,
	graph *kwcontext.ResourceGraph,
	incidentEngine *incident.Engine,
	monitors monitorComponents,
	now func() time.Time,
) optionalRuns {
	namespaces, watchAll := ctl.NamespaceScope()
	runs := optionalRuns{
		securityRun:     boot.securityMonitor.Start,
		controlPlaneRun: monitors.controlPlaneRun,
	}
	if monitors.tlsProcessor != nil {
		runs.tlsSweep = monitors.tlsProcessor.SweepTLSSecrets
	}
	runs.statusRun = configureStatusMonitor(
		runtime, ctl, graph, incidentEngine, boot.healthServer, now,
		boot.clients.Dynamic, boot.clients.Discovery,
	)
	runs.metricsRun = configureMetricsMonitor(
		runtime,
		ctl,
		boot.clients.Kubernetes,
		incidentEngine,
		ctl.OwnerResolver(),
		boot.clients.Dynamic,
	)
	runs.probeRun = configureProbeRunner(
		runtime,
		ctl,
		incidentEngine,
		boot.clients.Kubernetes,
		graph,
		now,
		boot.clients.HTTP,
		boot.clients.Resolver,
	)
	runs.storageRun = newStorageGraphRun(
		runtime, graph, incidentEngine, ctl, boot.healthServer,
		boot.clients.Dynamic, boot.clients.Discovery,
	)
	runs.networkRun = newNetworkGraphRun(
		runtime, graph, ctl, boot.healthServer,
		boot.clients.Dynamic, boot.clients.Discovery,
	)
	if runtime.KubeletTelemetryMonitor().Enabled {
		runs.kubeletRun = configureKubeletRun(
			runtime, boot, ctl, incidentEngine, namespaces, watchAll, now,
		)
	}
	return runs
}

func configureKubeletRun(
	runtime config.RuntimeConfig,
	boot *bootstrap,
	ctl *controller.Controller,
	incidentEngine *incident.Engine,
	namespaces []string,
	watchAll bool,
	now func() time.Time,
) func(context.Context) {
	monitor := kubeletmetrics.NewWithClock(
		boot.clients.Kubernetes,
		runtime.KubeletTelemetryMonitor(),
		incidentEngine,
		clock.Func(now),
	)
	var stateStore kubeletmetrics.StateStore
	if runtime.KubeletTelemetryMonitor().PersistState {
		stateStore = boot.persistence
	}
	if err := monitor.ConfigureSources(kubeletmetrics.Sources{
		Namespaces: namespaces, WatchAll: watchAll,
		Allowed: ctl.NamespaceAllowed, OwnerResolver: ctl.OwnerResolver(),
		PodLister: ctl.PodLister(), NodeLister: ctl.NodeLister(),
		StateStore: stateStore,
	}); err != nil {
		return nil
	}
	boot.healthServer.SetTelemetryLister(monitor)
	return monitor.Start
}
