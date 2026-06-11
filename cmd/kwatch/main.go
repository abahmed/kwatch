package main

import (
	"context"
	"flag"
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
	"github.com/abahmed/kwatch/internal/crdwatch"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/upgrader"
	"github.com/abahmed/kwatch/internal/version"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Short())
		os.Exit(0)
	}

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
	alertManager.SetSilences(cfg.Silences)
	if cfg.MaxRecentLogLines > 0 {
		alertManager.SetMaxLogLines(int(cfg.MaxRecentLogLines))
	}

	up := upgrader.NewUpgrader(&cfg.Upgrader, alertManager, sm.GetStateManager())
	go up.CheckUpdates(ctx)

	stateMgr := sm.GetStateManager()
	baseline := stateMgr.GetBaseline(ctx)

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
		Baseline:          baseline,
		Enricher:          &enricher.DefaultEnricher{SeverityByOwnerKind: cfg.SeverityByOwnerKind},
		EscalationEnabled:         cfg.Correlation.Escalation.Enabled,
		EscalationTiers:           cfg.Correlation.Escalation.Tiers,
		InhibitNodeSuppressesPods: cfg.Inhibition.NodeSuppressesPods,
		StormEnabled:              cfg.StormConfig.Enabled,
		StormThreshold:            cfg.StormConfig.Threshold,
		StormWindow:               time.Duration(cfg.StormConfig.WindowMinutes) * time.Minute,
		StormDigestInterval:       time.Duration(cfg.StormConfig.DigestIntervalMinutes) * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action != model.ActionSkip {
				alertManager.NotifyIncident(inc, action)
			}
		},
		OnBaselineChange: func(b map[string]int64) {
			if err := stateMgr.SaveBaseline(context.Background(), b); err != nil {
				klog.ErrorS(err, "failed to save baseline")
			}
		},
	})

	healthServer.SetIncidentAPI(correlator)
	healthServer.SetAlertManager(alertManager)

	pvcMonitor := pvc.NewPvcMonitor(k8sClient, &cfg.PvcMonitor, alertManager, correlator)
	hbMonitor := heartbeat.NewHeartbeatMonitor(&cfg.HeartbeatMonitor)

	h := handler.NewHandler(
		k8sClient,
		cfg,
		correlator,
		alertManager,
	)

	ctrl, cleanup := controller.New(k8sClient, cfg, h)
	defer cleanup()

	runLeaderTasks := func(ctx context.Context) {
		go correlator.StartCleanup(ctx)
		go pvcMonitor.Start(ctx)
		go hbMonitor.Start(ctx)
		if cfg.TlsMonitor.Enabled {
			go func() {
				h.SweepTLSSecrets()
				ticker := time.NewTicker(24 * time.Hour)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						h.SweepTLSSecrets()
					}
				}
			}()
		}
		if cfg.CrdConfig.Enabled {
			restCfg, err := client.GetRestConfig(&cfg.App)
			if err != nil {
				klog.ErrorS(err, "failed to get rest config for CRD watcher")
			} else {
				resync := time.Duration(cfg.ResyncSeconds) * time.Second
				w := crdwatch.New(cfg, alertManager, restCfg, k8s.GetNamespace(), resync)
				if err := w.Start(ctx); err != nil {
					klog.ErrorS(err, "CRD watcher error")
				}
			}
		}
		sm.NotifyStartup()
		workers := cfg.Workers
		if workers < 1 {
			workers = 1
		}
		if err := ctrl.Run(ctx, workers); err != nil {
			klog.ErrorS(err, "controller error")
		}
	}

	if cfg.LeaderElection.Enabled {
		leaseName := cfg.LeaderElection.LeaseName
		if leaseName == "" {
			leaseName = "kwatch-leader"
		}
		leaseNS := cfg.LeaderElection.Namespace
		if leaseNS == "" {
			leaseNS = k8s.GetNamespace()
		}
		podName := os.Getenv("HOSTNAME")
		if podName == "" {
			podName, _ = os.Hostname()
		}

		lock := &resourcelock.LeaseLock{
			LeaseMeta:  apiv1.ObjectMeta{Name: leaseName, Namespace: leaseNS},
			Client:     k8sClient.CoordinationV1(),
			LockConfig: resourcelock.ResourceLockConfig{Identity: podName},
		}

		go leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
			Lock:            lock,
			ReleaseOnCancel: true,
			LeaseDuration:   15 * time.Second,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					klog.InfoS("became leader, starting tasks")
					runLeaderTasks(ctx)
				},
				OnStoppedLeading: func() {
					klog.ErrorS(nil, "lost leadership, exiting for clean re-election")
					os.Exit(0)
				},
			},
		})
	} else {
		go runLeaderTasks(ctx)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	klog.InfoS("shutting down gracefully...")
	cancel()
	healthServer.Stop(ctx)
	os.Exit(0)
}
