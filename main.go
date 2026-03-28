package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/abahmed/kwatch/client"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/controller"
	"github.com/abahmed/kwatch/handler"
	"github.com/abahmed/kwatch/health"
	"github.com/abahmed/kwatch/pvcmonitor"
	"github.com/abahmed/kwatch/startup"
	"github.com/abahmed/kwatch/storage/memory"
	"github.com/abahmed/kwatch/upgrader"
	"github.com/abahmed/kwatch/util"
	"github.com/abahmed/kwatch/version"
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
		cfg.Alert,
		&cfg.App,
	)
	sm.HandleStartup(context.Background())

	healthServer := health.NewHealthServer(cfg.HealthCheck)
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

	ctrl, cleanup := controller.New(k8sClient, cfg, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := ctrl.Run(ctx, 1); err != nil {
			logrus.Fatalf("controller error: %s", err.Error())
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logrus.Info("shutting down gracefully...")
	cancel()
	healthServer.Stop(context.Background())
	os.Exit(0)
}
