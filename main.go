package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/pvcmonitor"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/storage/memory"
	"github.com/abahmed/kwatch/internal/upgrader"
	"github.com/abahmed/kwatch/internal/util"
	"github.com/abahmed/kwatch/internal/version"
	"github.com/abahmed/kwatch/internal/watcher"
	"github.com/sirupsen/logrus"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		logrus.Fatalf("failed to load config: %s", err.Error())
	}
	util.SetLogFormatter(cfg.App.LogFormatter)

	logrus.Info(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	k8sClient := client.Create(&cfg.App)

	sm := startup.NewStartupManager(
		k8sClient,
		util.GetNamespace(),
		&cfg.Telemetry,
		cfg.Alert,
		&cfg.App,
	)
	sm.HandleStartup(context.Background())

	healthServer := health.NewHealthServer(cfg.HealthCheck.Port, cfg.HealthCheck.Enabled)
	healthServer.Start(context.Background())

	upgrader := upgrader.NewUpgrader(&cfg.Upgrader, sm.GetAlertManager(), sm.GetStateManager())
	go upgrader.CheckUpdates()

	pvcMonitor := pvcmonitor.NewPvcMonitor(k8sClient, &cfg.PvcMonitor, sm.GetAlertManager())
	go pvcMonitor.Start()

	h := handler.NewHandler(
		k8sClient,
		cfg,
		memory.NewMemory(),
		sm.GetAlertManager(),
	)

	watcher.Start(k8sClient, cfg, h)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logrus.Info("shutting down gracefully...")
	healthServer.Stop(context.Background())
	os.Exit(0)
}
