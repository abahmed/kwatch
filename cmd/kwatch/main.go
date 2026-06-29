package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/crdwatch"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/upgrader"
	"github.com/abahmed/kwatch/internal/version"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Short())
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "lint":
			strict := false
			check := false
			for _, a := range args[1:] {
				if a == "--strict" || a == "strict" {
					strict = true
				}
				if a == "--check" || a == "check" {
					check = true
				}
			}
			runLint(strict, check)
			return
		case "replay":
			runReplay()
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		os.Exit(1)
	}

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	k8s.InitHTTPClient(&cfg.App)

	k8sClient := client.Create(&cfg.App)

	sm := startup.NewStartupManager(
		k8sClient,
		k8s.GetNamespace(),
		cfg.Alert,
		&cfg.App,
	)
	sm.HandleStartup(ctx)

	healthServer := health.NewHealthServer(cfg.HealthCheck)

	alertManager := sm.GetAlertManager()
	alertManager.SetSilences(cfg.Silences)
	alertManager.SetTemplates(cfg.Templates)
	if cfg.MaxRecentLogLines > 0 {
		alertManager.SetMaxLogLines(int(cfg.MaxRecentLogLines))
	}
	alertManager.SetLLM(cfg.LLM)
	alertManager.Start(ctx)

	up := upgrader.NewUpgrader(&cfg.Upgrader, alertManager, sm.GetStateManager())
	go up.CheckUpdates(ctx)

	stateMgr := sm.GetStateManager()
	baseline := stateMgr.GetBaseline(ctx)
	stateMgr.MigrateLegacyBaseline(ctx)

	baselineCh := make(chan map[string]map[string]int64, 64)
	go startBaselineSaver(ctx, stateMgr, baselineCh, 0)

	var correlator *correlation.Engine
	correlator = correlation.NewEngine(correlation.Config{
		Window:                     time.Duration(cfg.Correlation.Window) * time.Minute,
		LifecycleInterval:          time.Duration(cfg.Correlation.LifecycleInterval) * time.Minute,
		Baseline:                   baseline,
		Enricher:                   &enricher.DefaultEnricher{SeverityByOwnerKind: cfg.SeverityByOwnerKind, SeverityByReason: cfg.SeverityByReason},
		EscalationEnabled:          cfg.Correlation.Escalation.Enabled,
		EscalationTiers:            cfg.Correlation.Escalation.Tiers,
		InhibitNodeSuppressesPods:  cfg.Inhibition.NodeSuppressesPods,
		StormEnabled:               cfg.StormConfig.Enabled,
		StormThreshold:             cfg.StormConfig.Threshold,
		StormWindow:                time.Duration(cfg.StormConfig.WindowMinutes) * time.Minute,
		StormDigestInterval:        time.Duration(cfg.StormConfig.DigestIntervalMinutes) * time.Minute,
		RenotifyIntervalBySeverity: renotifyIntervalBySeverity(cfg.Correlation.Renotify.IntervalBySeverity),
		RenotifyMaxPerIncident:     cfg.Correlation.Renotify.MaxPerIncident,
		Runbooks:                   cfg.Runbooks,
		ResolveHoldDown:            time.Duration(cfg.Correlation.ResolveHoldDown) * time.Second,
		MaxBaseline:                cfg.Correlation.MaxBaseline,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action != model.ActionSkip {
				alertManager.NotifyIncident(inc, action)
			}
			metrics.Default.ActiveIncidents.Store(int64(correlator.ActiveCount()))
		},
		OnBaselineChange: func(b map[string]map[string]int64) {
			total := 0
			for _, pods := range b {
				total += len(pods)
			}
			metrics.Default.BaselineSize.Store(int64(total))
			select {
			case baselineCh <- b:
			default:
				// Channel full: drop oldest, keep newest
			}
		},
	})

	alertManager.SetAnalysisWriter(correlator.SetAnalysis)
	healthServer.SetIncidentAPI(correlator)
	healthServer.SetAlertManager(alertManager)
	healthServer.SetDeadLetterLister(alertManager)
	healthServer.Start(ctx)

	pvcMonitor := pvc.NewPvcMonitor(k8sClient, &cfg.PvcMonitor, alertManager, correlator, stateMgr)
	hbMonitor := heartbeat.NewHeartbeatMonitor(&cfg.HeartbeatMonitor)

	h := handler.NewHandler(
		k8sClient,
		cfg,
		correlator,
		alertManager,
	)
	if cfg.PvcMonitor.Enabled {
		h.SetPvcSampler(func(nodeName string) { go pvcMonitor.SampleNode(ctx, nodeName) })
	}

	ctrl, cleanup := controller.New(k8sClient, cfg, h)
	ctrl.SetReadyFunc(func() { healthServer.SetReady(true) })
	var cleanupOnce sync.Once
	cleanupSafe := func() { cleanupOnce.Do(cleanup) }
	defer cleanupSafe()

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
				w := crdwatch.New(cfg, alertManager, correlator, restCfg, k8s.GetNamespace(), resync)
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

	go runLeaderTasks(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	klog.InfoS("shutting down gracefully...")
	cancel()
	select {
	case <-alertManager.Done():
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for alert manager to drain")
	}
	shutdownCtx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	healthServer.SetReady(false)
	healthServer.Stop(shutdownCtx)
	sc()
	cleanupSafe()
	os.Exit(0)
}

func runLint(strict, check bool) {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	errs := config.ValidateConfig(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		os.Exit(1)
	}
	if strict {
		if err := config.LintStrict(); err != nil {
			fmt.Fprintf(os.Stderr, "STRICT ERROR: %v\n", err)
			os.Exit(1)
		}
	}
	if check {
		am := &alert.AlertManager{}
		am.Init(cfg.Alert, &cfg.App)
		results := am.VerifyAll()
		hasErr := false
		for name, err := range results {
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: FAIL — %v\n", name, err)
				hasErr = true
			} else {
				fmt.Printf("  %s: OK\n", name)
			}
		}
		if hasErr {
			os.Exit(1)
		}
	}
	fmt.Println("config OK")
}

func runReplay() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	providers := make([]string, 0, len(cfg.Alert))
	for k := range cfg.Alert {
		providers = append(providers, k)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: invalid event line: %v\n  %s\n", err, line)
			continue
		}
		msg := fmt.Sprintf("[replay] %s/%s %s: %s", ev.Namespace, ev.PodName, ev.Reason, ev.Events)
		fmt.Printf("would notify %v: %s\n", providers, msg)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: reading stdin: %v\n", err)
		os.Exit(1)
	}
}

// startBaselineSaver coalesces baseline writes: at most one ConfigMap write
// every interval. The latest snapshot always wins. Use 0 for the default
// interval (10 seconds).
func startBaselineSaver(ctx context.Context, stateMgr interface {
	SaveBaseline(context.Context, map[string]map[string]int64) error
}, ch <-chan map[string]map[string]int64, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	var pending map[string]map[string]int64
	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case b := <-ch:
			pending = b
			if timer == nil {
				timer = time.NewTimer(interval)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(interval)
			}
			timerC = timer.C
		case <-timerC:
			if err := stateMgr.SaveBaseline(context.Background(), pending); err != nil {
				klog.ErrorS(err, "failed to save baseline")
			}
			timerC = nil
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			if pending != nil {
				fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = stateMgr.SaveBaseline(fctx, pending)
				cancel()
			}
			return
		}
	}
}

func renotifyIntervalBySeverity(m map[string]int) map[string]time.Duration {
	r := make(map[string]time.Duration, len(m))
	for k, v := range m {
		r[k] = time.Duration(v) * time.Minute
	}
	return r
}
