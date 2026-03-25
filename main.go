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
	"github.com/abahmed/kwatch/internal/detector"
	"github.com/abahmed/kwatch/internal/detector/aggregator"
	"github.com/abahmed/kwatch/internal/detector/cluster"
	"github.com/abahmed/kwatch/internal/detector/store"
	"github.com/abahmed/kwatch/internal/detector/volume"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/startup"
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

	vol, err := volume.New(&volume.Config{
		BasePath:     "/data",
		SyncInterval: 30 * time.Second,
	})
	if err != nil {
		logrus.Warnf("Failed to create volume: %v, using memory fallback", err)
		vol, _ = volume.New(&volume.Config{
			BasePath:     "",
			SyncInterval: 30 * time.Second,
		})
	}

	s := store.NewStore(vol)

	pipelineCfg := &detector.PipelineConfig{
		Volume:          vol,
		Client:          k8sClient,
		Config:          cfg,
		DedupWindow:     5 * time.Minute,
		AggregateWindow: 10 * time.Minute,
	}

	pipeline := detector.NewPipeline(pipelineCfg)

	pipeline.AddDetector(detector.NewPodDetector())
	pipeline.AddDetector(detector.NewContainerDetector())
	pipeline.AddDetector(detector.NewNodeDetector())
	pipeline.AddDetector(detector.NewPVCUsageDetector(80.0))
	pipeline.AddDetector(detector.NewResourceDetector(&detector.ResourceConfig{
		CPUThreshold:    80.0,
		MemoryThreshold: 80.0,
	}))

	clusterDetector := cluster.NewDetector(&cluster.Config{
		ThresholdPercent: 5.0,
		MinThreshold:     5,
		MaxThreshold:     20,
		Window:           15 * time.Minute,
	})
	pipeline.AddDetector(clusterDetector)

	dedup := s.NewDeduplication()
	pipeline.SetDeduplication(dedup)

	agg := aggregator.NewAggregator(s)
	pipeline.SetAggregator(agg)

	logrus.Info("kwatch started with intelligent detection pipeline")

	h := detector.NewEventHandler(pipeline, sm.GetAlertManager(), k8sClient)

	watcher.Start(k8sClient, cfg, h)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logrus.Info("shutting down gracefully...")
	healthServer.Stop(context.Background())
	os.Exit(0)
}
