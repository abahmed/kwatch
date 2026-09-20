package app

import (
	"context"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/metricsapi"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

func configureMetricsMonitor(
	runtime config.RuntimeConfig,
	ctl *controller.Controller,
	clientset kubernetes.Interface,
	incidentSink monitor.ObservationSink,
	owners observe.OwnerResolver,
	dynamicClient dynamic.Interface,
) func(context.Context) error {
	if !runtime.Monitors().Metrics().Enabled {
		return nil
	}
	metricsMonitor := metricsapi.NewWithClient(
		dynamicClient, clientset, runtime.Monitors().Metrics(), incidentSink,
	)
	namespaces, watchAll := ctl.NamespaceScope()
	if err := metricsMonitor.ConfigureSources(metricsapi.Sources{
		PodLister: ctl.PodLister(), OwnerResolver: owners,
		Allowed: ctl.NamespaceAllowed, Namespaces: namespaces,
		WatchAll: watchAll,
	}); err != nil {
		return func(context.Context) error { return err }
	}
	return metricsMonitor.Start
}
