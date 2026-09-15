package app

import (
	"context"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/probe"
)

func configureProbeRunner(
	runtime config.RuntimeConfig,
	ctl *controller.Controller,
	incidentSink monitor.ObservationSink,
	clientset kubernetes.Interface,
	graph *kwcontext.ResourceGraph,
	now func() time.Time,
	httpClient *http.Client,
	resolver probe.HostResolver,
) func(context.Context) {
	probeConfig := runtime.ActiveProbeMonitor()
	if !probeConfig.Enabled || !activeProbesEnabled(probeConfig) {
		return nil
	}
	monitor := probe.NewWithDependencies(
		probeConfig,
		incidentSink,
		httpClient,
		resolver,
		clock.Func(now),
	)
	namespaces, watchAll := ctl.NamespaceScope()
	if err := monitor.ConfigureSources(probe.Sources{
		Kubernetes: clientset, ServiceLister: ctl.ServiceLister(),
		Graph: graph, Namespaces: namespaces, WatchAll: watchAll,
		Allowed: ctl.NamespaceAllowed,
	}); err != nil {
		return nil
	}
	return monitor.Start
}

func activeProbesEnabled(cfg config.ActiveProbeMonitor) bool {
	return len(cfg.HTTP) > 0 || len(cfg.TCP) > 0 || len(cfg.DNS) > 0 ||
		cfg.AutoServices
}
