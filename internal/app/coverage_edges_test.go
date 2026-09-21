package app

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/delivery"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/persistence"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
)

type coverageLock struct {
	record   resourcelock.LeaderElectionRecord
	identity string
	events   []string
	err      error
}

func (l *coverageLock) Get(
	context.Context,
) (*resourcelock.LeaderElectionRecord, []byte, error) {
	return &l.record, nil, l.err
}

func (l *coverageLock) Create(
	context.Context, resourcelock.LeaderElectionRecord,
) error {
	return l.err
}

func (l *coverageLock) Update(
	context.Context, resourcelock.LeaderElectionRecord,
) error {
	return l.err
}

func (l *coverageLock) RecordEvent(event string) {
	l.events = append(l.events, event)
}

func (l *coverageLock) Identity() string { return l.identity }

func (l *coverageLock) Describe() string { return "coverage lock" }

func TestRenewalLockAndIdentityHelpers(t *testing.T) {
	when := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	delegate := &coverageLock{identity: "pod-a"}
	var renewed time.Time
	lock := &renewalTrackingLock{
		delegate:  delegate,
		onRenewal: func(got time.Time) { renewed = got },
	}
	if _, _, err := lock.Get(context.Background()); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if err := lock.Create(context.Background(), resourcelock.LeaderElectionRecord{
		RenewTime: metav1.NewTime(when),
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := lock.Update(context.Background(), resourcelock.LeaderElectionRecord{
		RenewTime: metav1.NewTime(when.Add(time.Minute)),
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	lock.RecordEvent("renewed")
	if lock.Identity() != "pod-a" || lock.Describe() != "coverage lock" ||
		len(delegate.events) != 1 {
		t.Fatal("lock delegate methods were not forwarded")
	}
	if !renewed.Equal(when.Add(time.Minute)) {
		t.Fatalf("renewal = %v", renewed)
	}
	lock.onRenewal = nil
	lock.recordRenewal(resourcelock.LeaderElectionRecord{})
	delegate.err = errors.New("update failed")
	if err := lock.Update(
		context.Background(), resourcelock.LeaderElectionRecord{},
	); err == nil {
		t.Fatal("Update() hid delegate error")
	}

	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "custom-leader")
	if electionLeaseName() != "custom-leader" {
		t.Fatal("explicit election name was ignored")
	}
	t.Setenv("KWATCH_LEADER_ELECTION_NAME", "")
	t.Setenv("KWATCH_INSTALLATION_ID", "installation")
	if electionLeaseName() != "installation-leader" {
		t.Fatal("installation election name was ignored")
	}
	t.Setenv("POD_NAME", "pod-a")
	if got, err := podIdentity(); err != nil || got != "pod-a" {
		t.Fatalf("podIdentity() = %q, %v", got, err)
	}
}

func TestAppBaselineAndPersistenceStatusHelpers(t *testing.T) {
	baselineCh := make(chan map[string]map[string]int64, 1)
	onChange := onBaselineChange(baselineCh)
	first := map[string]map[string]int64{"apps": {"pod": 1}}
	second := map[string]map[string]int64{"apps": {"pod": 2}}
	onChange(first)
	onChange(second)
	if got := <-baselineCh; got["apps"]["pod"] != 2 {
		t.Fatal("baseline channel did not retain newest snapshot")
	}
	if writesAllowed(nil) != true || writesAllowed(func() bool { return false }) {
		t.Fatal("writesAllowed() returned the wrong gate state")
	}
	if ctx, cancel := finalWriteContext(nil); ctx == nil || cancel == nil {
		t.Fatal("finalWriteContext(nil) returned an invalid context")
	} else {
		cancel()
	}

	healthServer := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	status := persistenceStatus(healthServer, "store", false, nil)
	status(nil)
	status(errors.New("write failed"))
	if _, ok := healthServer.ComponentErrors()["store"]; !ok {
		t.Fatal("persistence status did not update health")
	}
}

func TestAppRestoreRecordHelpers(t *testing.T) {
	client := fake.NewSimpleClientset()
	manager := persistence.NewManagerWithClock(
		client, "kwatch", clock.RealClock{},
	)
	manager.BeginMigrationReport()
	recordOptionalRestore(manager, "feedback", nil)
	recordOptionalRestore(manager, "changes", errors.New("missing"))
	recordRequiredRestore(manager, "baseline", nil)
	recordRequiredRestore(manager, "pvc", errors.New("missing"))
	recordNotRequiredRestore(manager, "telemetry")
	if got := manager.MigrationReport(); len(got.Operations) != 5 {
		t.Fatalf("migration results = %d", len(got.Operations))
	}

	tracker := kwcontext.NewChangeTrackerWithClock(10, clock.RealClock{})
	if err := restoreChangeTracker(
		context.Background(), manager, tracker,
	); err != nil {
		t.Fatalf("restoreChangeTracker() error = %v", err)
	}
	feedback := insight.NewFeedbackStoreWithClock(clock.RealClock{})
	if err := restoreFeedbackStore(
		context.Background(), manager, feedback,
	); err != nil {
		t.Fatalf("restoreFeedbackStore() error = %v", err)
	}
	feedbackSnapshotSaver(make(chan []insight.RCARecord, 1), feedback)()
}

func TestAppPersistenceSetupStartsOwnedSavers(t *testing.T) {
	manager := persistence.NewManagerWithClock(
		fake.NewSimpleClientset(), "kwatch", clock.RealClock{},
	)
	healthServer := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	setup := configurePersistence(
		manager, config.RuntimeConfig{}, now, healthServer, nil,
	)
	if setup.tracker == nil || setup.feedbackStore == nil ||
		setup.saveFeedback == nil || setup.start == nil {
		t.Fatal("persistence setup omitted required collaborators")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	supervisor := newComponentSupervisor(now)
	setup.start(ctx, supervisor, func() bool { return false })
	waitForSupervisor(supervisor)
	if err := setup.activate(context.Background(), nil); err != nil {
		t.Fatalf("persistence activation failed: %v", err)
	}
	recordRestoreResult(manager, "engine", nil)
	recordRestoreResult(manager, "engine", errors.New("restore failed"))
}

func TestAppRuntimeHelperBranches(t *testing.T) {
	if deliveryProgress(nil) != nil {
		t.Fatal("nil dependencies returned delivery progress")
	}
	deps := &serverDeps{}
	if len(activeOptionalComponents(deps)) == 0 {
		t.Fatal("optional component inventory was empty")
	}
	if monitoredRun(deps, "test", nil).run != nil {
		t.Fatal("nil optional runner was retained")
	}
	if waitForActiveSession(closedChan()) != nil {
		t.Fatal("closed active session did not complete")
	}
	waitForSupervisor(newComponentSupervisor(time.Now))
	if got := describeDependency("bad"); got != "bad" {
		t.Fatal("malformed dependency changed")
	}
	_ = model.ObjectRef{}
	_ = corev1.Namespace{}
}

func TestAppSaverLifecycleHelpers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	supervisor := newComponentSupervisor(time.Now)
	doneRequired := make(chan struct{})
	doneOptional := make(chan struct{})
	startRequiredSaver(ctx, supervisor, "required", doneRequired,
		time.Now, nil, func(context.Context, func()) error { return nil }, nil)
	startOptionalSaver(ctx, supervisor, "optional", doneOptional,
		time.Now, nil, func(context.Context, func()) error { return nil })
	waitForSupervisor(supervisor)
	if !waitPersistenceComponent(doneRequired, "required") ||
		!waitPersistenceComponent(doneOptional, "optional") {
		t.Fatal("saver did not complete")
	}
	if !waitFeedbackSaver(&serverDeps{}) ||
		!waitIncidentSaver(&serverDeps{}) {
		t.Fatal("nil saver channels were not treated as complete")
	}
	manager := persistence.NewManagerWithClock(
		fake.NewSimpleClientset(), "kwatch", clock.RealClock{},
	)
	startFeedbackSaver(ctx, manager, make(chan []insight.RCARecord),
		nil, func() bool { return true }, nil)
	if err := startIncidentSaver(ctx, manager, make(chan stateSnapshot),
		nil, func() bool { return true }, nil); err != nil {
		t.Fatalf("incident saver returned error: %v", err)
	}
	if err := startChangeHistorySaver(ctx, manager,
		kwcontext.NewChangeTrackerWithClock(1, clock.RealClock{}), nil,
		func() bool { return true }, nil); err != nil {
		t.Fatalf("change history saver returned error: %v", err)
	}
}

func TestAppIncidentEngineAndMassFailureHelpers(t *testing.T) {
	manager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}},
	)
	runtime := config.RuntimeConfigFor(&config.Config{})
	engine := newIncidentEngine(
		runtime, time.Now, nil, nil, manager,
		audit.NewLogger(audit.Config{Now: time.Now}),
		kwcontext.NewResourceGraph(), make(chan map[string]map[string]int64, 1),
		nil, nil, nil,
	)
	if engine == nil {
		t.Fatal("newIncidentEngine returned nil")
	}
	holder := &engineHolder{engine: engine}
	current := map[string]insight.MassFailure{
		"service/apps/api": {
			SharedDependency: "service/apps/api", AffectedCount: 3,
			Threshold: 3, Reason: "unavailable", Namespace: "apps",
			ResourceKind: "service",
		},
	}
	notifyNewMassFailures(holder, current)
	notifyNewMassFailures(holder, current)
	resolveClearedMassFailures(holder, nil)
	massFailureHook(&engineOptions{graph: kwcontext.NewResourceGraph()}, holder)()
	_ = dependencyResolver(nil)(&model.Incident{})
}

func TestAppRestoreEmptyStateHelpers(t *testing.T) {
	manager := persistence.NewManagerWithClock(
		fake.NewSimpleClientset(), "kwatch", clock.RealClock{},
	)
	engine := incident.NewEngineWithClock(incident.Config{}, clock.RealClock{})
	if _, err := restoreIncidents(
		context.Background(), manager, engine, nil,
	); err != nil {
		t.Fatalf("restoreIncidents() error = %v", err)
	}
	if err := restoreGroups(context.Background(), manager, engine); err != nil {
		t.Fatalf("restoreGroups() error = %v", err)
	}
}

func TestAppDisabledRuntimeBranches(t *testing.T) {
	runtime := config.RuntimeConfigFor(&config.Config{})
	if configureMetricsMonitor(runtime, nil, nil, nil, nil, nil) != nil {
		t.Fatal("disabled metrics monitor returned a runner")
	}
	if configureProbeRunner(runtime, nil, nil, nil, nil, time.Now,
		nil, nil) != nil {
		t.Fatal("disabled probe monitor returned a runner")
	}
	if configureStatusMonitor(runtime, nil, nil, nil, nil, time.Now,
		nil, nil) != nil {
		t.Fatal("disabled status monitor returned a runner")
	}
	if run, monitor := configureControlPlaneMonitor(
		runtime, nil, nil, nil, nil, time.Now,
	); run != nil || monitor != nil {
		t.Fatal("disabled control-plane monitor returned a runtime")
	}
	if configureSecurityMonitor(
		runtime, fake.NewSimpleClientset(), time.Now,
	) == nil {
		t.Fatal("security monitor was not composed")
	}
	if err := applyStartupCRD(
		context.Background(), &config.Config{}, nil,
	); err != nil {
		t.Fatalf("disabled startup CRD returned error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := incident.NewEngineWithClock(incident.Config{}, clock.RealClock{})
	if err := runIncidentCleanup(
		ctx, &serverDeps{incidentEngine: engine},
	); err != nil {
		t.Fatalf("incident cleanup returned error: %v", err)
	}
	manager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}},
	)
	if err := runDelivery(ctx, &serverDeps{deliveryManager: manager}); err != nil {
		t.Fatalf("delivery returned error: %v", err)
	}
	heartbeatMonitor := heartbeat.NewHeartbeatMonitor(nil, nil)
	if err := runHeartbeat(
		ctx, &serverDeps{hbMonitor: heartbeatMonitor},
	); err != nil {
		t.Fatalf("heartbeat returned error: %v", err)
	}
	if err := runNamespaceScopeWatcher(ctx, &serverDeps{
		ctl: &controller.Controller{}, clients: client.ClientSet{},
		runtime: runtime,
	}); err != nil {
		t.Fatalf("namespace watcher returned error: %v", err)
	}
	if configureTelemetryRunner(
		config.Telemetry{}, nil, "", "", time.Now, nil, nil,
	) != nil {
		t.Fatal("disabled telemetry returned a runner")
	}
}

func TestAppShutdownAndRestoreWrappers(t *testing.T) {
	controllerDone := make(chan struct{})
	close(controllerDone)
	manager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}},
	)
	deps := &serverDeps{
		clients: client.ClientSet{
			Clock: clock.RealClock{},
		},
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{},
		),
		deliveryManager: manager, persistenceGate: newPersistenceGate(),
		cleanup: func() {}, cancel: func() {},
		runtime:        config.RuntimeConfigFor(&config.Config{}),
		controllerDone: controllerDone,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = serve(ctx, deps)
	if err := runLeaderElection(context.Background(), &serverDeps{}); err == nil {
		t.Fatal("invalid leader election dependencies were accepted")
	}
	if err := runLeaderElectionWithFactory(
		context.Background(), &serverDeps{}, nil,
	); err == nil {
		t.Fatal("invalid leader election factory was accepted")
	}
	if err := restoreProviderThreads(
		context.Background(), persistence.NewManagerWithClock(
			fake.NewSimpleClientset(), "kwatch", clock.RealClock{},
		), nil,
		incident.NewEngineWithClock(incident.Config{}, clock.RealClock{}), nil,
	); err != nil {
		t.Fatalf("nil thread restorer returned error: %v", err)
	}
	if err := restoreEngineState(
		context.Background(), persistence.NewManagerWithClock(
			fake.NewSimpleClientset(), "kwatch", clock.RealClock{},
		), incident.NewEngineWithClock(incident.Config{}, clock.RealClock{}),
	); err != nil {
		t.Fatalf("restoreEngineState() error = %v", err)
	}
	_, err := newKubernetesElection(leaderelection.LeaderElectionConfig{
		Lock:          &coverageLock{identity: "pod"},
		LeaseDuration: leaderLeaseDuration,
		RenewDeadline: leaderRenewDeadline,
		RetryPeriod:   leaderRetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(context.Context) {},
			OnStoppedLeading: func() {},
			OnNewLeader:      func(string) {},
		},
	})
	if err != nil {
		t.Fatalf("newKubernetesElection() error = %v", err)
	}
}

func TestAppOptionalCompositionAndControllerRuntime(t *testing.T) {
	runtime := config.RuntimeConfigFor(&config.Config{})
	clientset := fake.NewSimpleClientset()
	healthServer := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	manager := persistence.NewManagerWithClock(
		clientset, "kwatch", clock.RealClock{},
	)
	deliveryManager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}},
	)
	boot := &bootstrap{
		runtime: runtime, persistence: manager, healthServer: healthServer,
		securityMonitor: configureSecurityMonitor(runtime, clientset, time.Now),
		deliveryManager: deliveryManager,
		clients: client.ClientSet{
			Kubernetes: clientset, Clock: clock.RealClock{},
		}, clock: clock.RealClock{},
	}
	engine := incident.NewEngineWithClock(incident.Config{}, clock.RealClock{})
	ctl := &controller.Controller{}
	graph := kwcontext.NewResourceGraph()
	optional := configureOptionalRuns(
		runtime, boot, ctl, graph, engine, monitorComponents{}, time.Now,
	)
	if optional.securityRun == nil {
		t.Fatal("security run was not configured")
	}
	pvcMonitor := pvc.NewPvcMonitorWithRuntimeAndClock(
		clientset, runtime, engine, manager, clock.RealClock{},
	)
	if err := configureControllerRuntime(
		runtime, boot, ctl, pvcMonitor,
	); err != nil {
		t.Fatalf("configureControllerRuntime() error = %v", err)
	}
	components := composeMonitorComponents(
		runtime, clientset, boot.clients, engine, deliveryManager, time.Now,
	)
	created, cleanup, err := newMonitorController(
		clientset, runtime, components, controller.RuntimeDependencies{
			Context: context.Background(), Graph: graph, Now: time.Now,
		},
	)
	if err != nil {
		t.Fatalf("newMonitorController() error = %v", err)
	}
	if created == nil {
		t.Fatal("newMonitorController() returned nil controller")
	}
	initialized := make(chan struct{})
	close(initialized)
	activeCtx, stopActive := context.WithCancel(context.Background())
	stopActive()
	activeDeps := &serverDeps{
		clients: client.ClientSet{Clock: clock.RealClock{}},
		runtime: runtime, healthServer: healthServer,
		deliveryManager: deliveryManager, incidentEngine: engine,
		pvcMonitor: pvcMonitor,
		hbMonitor:  heartbeat.NewHeartbeatMonitor(nil, nil), ctl: created,
		initialized: initialized, persistenceGate: newPersistenceGate(),
		controllerDone: make(chan struct{}), incidentCh: make(chan stateSnapshot, 1),
		notifyStartup:      func() {},
		controllerProgress: newComponentProgress(time.Now()),
	}
	if err := runActiveComponents(activeCtx, activeDeps); err != nil {
		t.Fatalf("runActiveComponents() error = %v", err)
	}
	cleanup()
	if err := restoreControllerRuntime(
		context.Background(), boot, ctl, engine,
	); err != nil {
		t.Fatalf("restoreControllerRuntime() error = %v", err)
	}
	deps := makeServerDeps(
		context.Background(), func() {}, runtime, boot, ctl, func() {},
		persistenceSetup{}, engine, pvcMonitor,
		heartbeat.NewHeartbeatMonitor(nil, nil), optional,
		audit.NewLogger(audit.Config{Now: time.Now}), nil, nil, nil,
	)
	if deps == nil || deps.persistenceGate == nil {
		t.Fatal("makeServerDeps() omitted lifecycle state")
	}
	boot.securityMonitor = configureSecurityMonitor(
		runtime, clientset, time.Now,
	)
	built, err := buildServerDeps(
		context.Background(), func() {}, runtime, boot, time.Now,
	)
	if err != nil {
		t.Fatalf("buildServerDeps() error = %v", err)
	}
	built.cleanup()
}

func TestAppEnabledOptionalConfigurationBranches(t *testing.T) {
	cfg := &config.Config{
		RuntimeMetricsMonitor: config.RuntimeMetricsMonitor{
			Enabled: true, IntervalSeconds: 1,
		},
		ActiveProbeMonitor: config.ActiveProbeMonitor{
			Enabled: true, AutoServices: true,
		},
		ClusterResourceMonitor: config.ClusterResourceMonitor{Enabled: true},
		ControlPlaneMonitor:    config.ControlPlaneMonitor{Enabled: true},
		KubeletTelemetryMonitor: config.KubeletTelemetryMonitor{
			Enabled: true, IntervalSeconds: 1,
		},
	}
	runtime := config.RuntimeConfigFor(cfg)
	ctl := &controller.Controller{}
	clientset := fake.NewSimpleClientset()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		apiruntime.NewScheme(),
		map[schema.GroupVersionResource]string{},
	)
	healthServer := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	if configureMetricsMonitor(
		runtime, ctl, clientset, nil, nil, dynamicClient,
	) == nil {
		t.Fatal("enabled metrics monitor returned no runner")
	}
	if configureProbeRunner(
		runtime, ctl, nil, clientset, kwcontext.NewResourceGraph(),
		time.Now, nil, nil,
	) == nil {
		t.Fatal("enabled probe monitor returned no runner")
	}
	if configureStatusMonitor(
		runtime, ctl, kwcontext.NewResourceGraph(), nil,
		healthServer, time.Now, dynamicClient, clientset.Discovery(),
	) == nil {
		t.Fatal("enabled status monitor returned no runner")
	}
	if run, monitor := configureControlPlaneMonitor(
		runtime, clientset, nil, nil, nil, time.Now,
	); run == nil || monitor == nil {
		t.Fatal("enabled control-plane monitor was not composed")
	}
	boot := &bootstrap{
		runtime:         runtime,
		healthServer:    healthServer,
		securityMonitor: configureSecurityMonitor(runtime, clientset, time.Now),
		clients: client.ClientSet{
			Kubernetes: clientset, Dynamic: dynamicClient,
			Discovery: clientset.Discovery(), Clock: clock.RealClock{},
		},
	}
	runs := configureOptionalRuns(
		runtime, boot, ctl, kwcontext.NewResourceGraph(),
		incident.NewEngineWithClock(incident.Config{}, clock.RealClock{}),
		monitorComponents{}, time.Now,
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, run := range []func(context.Context) error{
		runs.metricsRun, runs.probeRun, runs.statusRun, runs.storageRun,
		runs.networkRun,
	} {
		if run != nil {
			_ = run(ctx)
		}
	}
	_ = runs
}

func TestBootstrapActivationUsesStartupStateOnce(t *testing.T) {
	runtime := config.RuntimeConfigFor(&config.Config{})
	manager := persistence.NewManagerWithClock(
		fake.NewSimpleClientset(), "kwatch", clock.RealClock{},
	)
	boot := &bootstrap{
		runtime: runtime, persistence: manager,
		startupManager: startup.NewStartupManagerWithRuntime(
			manager, runtime, clock.RealClock{},
		),
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{},
		), clock: clock.RealClock{}, telemetryStatus: newAdoptionTelemetryStatus(),
	}
	if err := boot.activate(context.Background()); err != nil {
		t.Fatalf("bootstrap activation failed: %v", err)
	}
	if err := boot.activate(context.Background()); err != nil {
		t.Fatalf("second bootstrap activation failed: %v", err)
	}
}

func TestAppLoadsDefaultAndExplicitConfiguration(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	if cfg, err := loadConfig(); err != nil || cfg == nil {
		t.Fatalf("loadConfig() = %v, %v", cfg, err)
	}
	path := t.TempDir() + "/kwatch.yaml"
	if err := os.WriteFile(
		path, []byte("namespaces: [apps]\n"), 0600,
	); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	t.Setenv("CONFIG_FILE", path)
	cfg, err := loadConfig()
	if err != nil || cfg == nil || len(cfg.Namespaces) != 1 {
		t.Fatalf("explicit loadConfig() = %v, %v", cfg, err)
	}
}

func TestAppEntryAndShutdownGuardBranches(t *testing.T) {
	badPath := t.TempDir() + "/bad.yaml"
	if err := os.WriteFile(badPath, []byte("[bad"), 0600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	t.Setenv("CONFIG_FILE", badPath)
	if code := RunWithClock(time.Now); code != 1 {
		t.Fatalf("RunWithClock() code = %d, want config failure", code)
	}
	_, _ = newBootstrap(context.Background(), &config.Config{}, time.Now)
	closeAuditLogger(audit.NewLogger(audit.Config{Now: time.Now}))
	recordShutdownTimeout("coverage")
	saveFinalIncidentSnapshot(context.Background(), &serverDeps{})
	full := make(chan []insight.RCARecord, 1)
	full <- nil
	trySendFeedbackSnapshot(full, nil)
	done := make(chan struct{})
	close(done)
	if !waitFeedbackSaver(&serverDeps{feedbackDone: done}) ||
		!waitIncidentSaver(&serverDeps{incidentDone: done}) {
		t.Fatal("completed saver was not observed")
	}
}

func closedChan() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
