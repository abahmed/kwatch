package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/knowledge/store"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/upgrader"
)

// defaultDataDir holds the state file when KWATCH_DATA_DIR is unset. The
// deployment mounts the kwatch data volume here.
const defaultDataDir = "/var/lib/kwatch"

func dataDir() string {
	if dir := os.Getenv("KWATCH_DATA_DIR"); dir != "" {
		return dir
	}
	return defaultDataDir
}

// runActiveComponents is the leader session: it claims the state file,
// runs startup bookkeeping and supervises every active component until
// leadership or the application ends.
func runActiveComponents(ctx context.Context, deps *serverDeps) error {
	activeCtx, cancel := context.WithCancel(applicationContext(deps))
	defer cancel()
	go fenceOnLeadershipLoss(ctx, activeCtx, cancel)

	state, err := openStore(activeCtx, deps)
	if err != nil {
		deps.healthServer.SetComponentError("state", err)
		return fmt.Errorf("state: %w", err)
	}
	defer closeStore(state)
	disk := diskState{store: state, client: deps.clients.Kubernetes}
	session := startup.NewStartupManagerWithRuntime(
		disk, deps.runtime, deps.clients.Clock,
		newKubernetesRestartEvidence(
			deps.clients.Kubernetes, k8s.GetNamespace()),
	)
	result, err := session.Start(activeCtx)
	if err != nil {
		deps.healthServer.SetComponentError("state", err)
		return fmt.Errorf("state: startup: %w", err)
	}
	deps.readiness.setCurrent("state", true)
	if msg, ok := session.StartupMessage(); ok {
		deps.deliveryManager.Notify(msg)
	}

	supervisor := newComponentSupervisor(deps.clients.Clock.Now)
	startActiveComponents(activeCtx, deps, supervisor, activeResources{
		state: state, disk: disk, result: result, session: session,
	})
	select {
	case err = <-supervisor.errCh:
	case <-ctx.Done():
	}
	cancel()
	waitForSupervisor(supervisor)
	reason := "graceful_shutdown"
	if err != nil {
		reason = "internal_failure"
	}
	session.EndSession(context.WithoutCancel(ctx), reason)
	return err
}

// activeResources are what the leader session hands its components.
type activeResources struct {
	state   *store.Store
	disk    diskState
	result  startup.Result
	session *startup.StartupManager
}

func startActiveComponents(
	ctx context.Context,
	deps *serverDeps,
	supervisor *componentSupervisor,
	res activeResources,
) {
	supervisor.startOwned(ctx, componentSpec{
		name:     "delivery",
		required: true,
		progress: deliveryProgress(deps),
		onHealthy: func() {
			deps.readiness.setCurrent("delivery", true)
		},
		run: func(ctx context.Context) error {
			return runDelivery(ctx, deps)
		},
	})
	supervisor.startOwned(ctx, componentSpec{
		name:     "core",
		required: true,
		progress: deps.coreProgress,
		run: func(ctx context.Context) error {
			return runCore(ctx, deps, res.state)
		},
	})
	upgraderConfig := deps.runtime.Lifecycle().Upgrader()
	optional := []componentSpec{
		monitoredRun(deps, "telemetry", configureTelemetryRunner(
			deps.runtime.Lifecycle().Telemetry(), res.disk,
			res.result.ClusterID, res.result.CurrentVersion,
			deps.clients.Clock.Now, deps.clients.HTTP,
			deps.telemetryStatus,
		)),
		monitoredRun(deps, "upgrader", upgrader.NewUpgrader(
			&upgraderConfig, deps.deliveryManager, res.disk,
			deps.clients.HTTP,
		).CheckUpdates),
		monitoredRun(deps, "rbac", deps.securityMonitor.Start),
		monitoredComponent(deps, "heartbeat", runHeartbeat),
		monitoredRun(deps, "alive", aliveRecorder(res.session)),
	}
	if deps.runtime.Lifecycle().CRDEnabled() {
		optional = append(optional,
			monitoredComponent(deps, "crd-watcher", runCRDWatcher))
	}
	for _, component := range optional {
		supervisor.startOptional(ctx, component)
	}
}

// openStore opens the state file and claims it with this leadership term,
// so a deposed leader can no longer write.
func openStore(ctx context.Context, deps *serverDeps) (*store.Store, error) {
	s, err := store.Open(filepath.Join(dataDir(), "state.db"),
		store.Options{Now: deps.clients.Clock.Now})
	if err != nil {
		return nil, err
	}
	epoch, err := leaseEpoch(ctx, deps)
	if err == nil {
		err = s.Claim(epoch)
	}
	if err != nil {
		closeStore(s)
		return nil, err
	}
	return s, nil
}

func closeStore(s *store.Store) {
	if err := s.Close(); err != nil {
		klog.ErrorS(err, "close state store", "component", "state")
	}
}

// leaseEpoch numbers leadership terms from the Lease transition count,
// which increases with every new holder.
func leaseEpoch(ctx context.Context, deps *serverDeps) (uint64, error) {
	lease, err := electionClient(deps.clients).CoordinationV1().
		Leases(k8s.GetNamespace()).Get(ctx, electionLeaseName(),
		metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("read lease epoch: %w", err)
	}
	transitions := int32(0)
	if lease.Spec.LeaseTransitions != nil {
		transitions = *lease.Spec.LeaseTransitions
	}
	return uint64(transitions) + 1, nil
}
