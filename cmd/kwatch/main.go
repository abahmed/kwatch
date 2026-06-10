package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/upgrader"
	"github.com/abahmed/kwatch/internal/version"
	"k8s.io/klog/v2"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		os.Exit(1)
	}

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	k8sClient := client.Create(&cfg.App)

	sm := startup.NewStartupManager(
		k8sClient,
		k8s.GetNamespace(),
		cfg.Alert,
		&cfg.App,
	)
	sm.HandleStartup(ctx)

	healthServer := health.NewHealthServer(cfg.HealthCheck)
	healthServer.Start(ctx)

	alertManager := sm.GetAlertManager()

	up := upgrader.NewUpgrader(&cfg.Upgrader, alertManager, sm.GetStateManager())
	go up.CheckUpdates(ctx)

	startupQuiet := cfg.Correlation.StartupQuiet
	if startupQuiet <= 0 {
		startupQuiet = 30
	}

	correlator := correlation.NewEngine(correlation.Config{
		Window:            time.Duration(cfg.Correlation.Window) * time.Minute,
		Cooldown:          time.Duration(cfg.Correlation.Cooldown) * time.Minute,
		StaleThreshold:    time.Duration(cfg.Correlation.StaleThreshold) * time.Minute,
		LifecycleInterval: time.Duration(cfg.Correlation.LifecycleInterval) * time.Minute,
		StartupQuiet:      time.Duration(startupQuiet) * time.Second,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action != model.ActionSkip {
				alertManager.NotifyIncident(inc, action)
			}
		},
	})
	go correlator.StartCleanup(ctx)

	pvcMonitor := pvc.NewPvcMonitor(k8sClient, &cfg.PvcMonitor, alertManager, correlator)
	go pvcMonitor.Start(ctx)

	h := handler.NewHandler(
		k8sClient,
		cfg,
		correlator,
		alertManager,
	)

	ctrl, cleanup := controller.New(k8sClient, cfg, h)
	defer cleanup()

	go func() {
		if err := ctrl.Run(ctx, 1); err != nil {
			klog.ErrorS(err, "controller error")
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	klog.InfoS("shutting down gracefully...")
	cancel()
	healthServer.Stop(ctx)
	os.Exit(0)
}
