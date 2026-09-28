package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/core"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/knowledge/store"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
	"github.com/abahmed/kwatch/internal/signal/detect"
	"github.com/abahmed/kwatch/internal/story"
)

// coreV2Enabled selects the problem-centric core. It stays opt-in until it
// beats the current engine on the scorecard (ADR 0010, rollout).
func coreV2Enabled() bool {
	return os.Getenv("KWATCH_CORE") == "v2"
}

// defaultDataDir holds the state file when KWATCH_DATA_DIR is unset. The
// deployment mounts the kwatch data volume here.
const defaultDataDir = "/var/lib/kwatch"

func dataDir() string {
	if dir := os.Getenv("KWATCH_DATA_DIR"); dir != "" {
		return dir
	}
	return defaultDataDir
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
		_ = s.Close()
		return nil, err
	}
	return s, nil
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

// modelPruneInterval and modelRetention bound in-memory change history.
const (
	modelPruneInterval = 10 * time.Minute
	modelRetention     = 2 * time.Hour
)

// startCoreV2 runs the new core in place of the controller, incident
// engine and ConfigMap persistence. Delivery is already started.
func startCoreV2(
	ctx context.Context, deps *serverDeps, supervisor *componentSupervisor,
) {
	if deps.readiness != nil {
		for _, name := range []string{
			"restore", "persistence-writers", "incident",
		} {
			deps.readiness.setCurrent(name, true)
		}
	}
	supervisor.startOwned(ctx, componentSpec{
		name:     "core",
		required: true,
		progress: deps.controllerProgress,
		run: func(ctx context.Context) error {
			return runCoreV2(ctx, deps)
		},
	})
}

func runCoreV2(ctx context.Context, deps *serverDeps) error {
	appClock := deps.clients.Clock
	state, err := openStore(ctx, deps)
	if err != nil {
		return fmt.Errorf("open state store: %w", err)
	}
	defer func() {
		if err := state.Close(); err != nil {
			klog.ErrorS(err, "close state store", "component", "core")
		}
	}()
	model := knowledge.NewModel(knowledge.Options{})
	var source *kube.Source
	synced := func(kind knowledge.Kind) bool {
		return source != nil && source.Synced(kind)
	}
	engine, err := core.NewEngine(core.Dependencies{
		Model:     model,
		Detectors: newDetectorRegistry(synced),
		Problems:  problem.NewManager(problem.Config{}, newReasoner()),
		Sink: func(_ context.Context, d problem.Decision, m story.Message) {
			klog.InfoS("core decision", "component", "core",
				"problem", d.Problem.ID, "reason", d.Reason)
			deps.deliveryManager.Notify(story.Text(m))
		},
		Clock: coreClock{deps.clients.Clock},
		Store: core.NewProblemStore(state),
		Progress: func() {
			deps.controllerProgress.Touch(appClock.Now())
		},
	})
	if err != nil {
		return err
	}
	source, err = kube.NewSource(kube.SourceConfig{
		Client: deps.clients.Kubernetes,
		Resync: deps.runtime.Lifecycle().ResyncInterval(),
		Now:    appClock.Now,
		Submit: engine.Submit,
	})
	if err != nil {
		return err
	}
	var background sync.WaitGroup
	background.Add(3)
	go func() {
		defer background.Done()
		source.Run(ctx)
	}()
	go func() {
		defer background.Done()
		pruneModel(ctx, model, appClock.Now)
	}()
	go func() {
		defer background.Done()
		if source.WaitForSync(ctx) && deps.readiness != nil {
			deps.readiness.setCurrent("controller", true)
		}
	}()
	defer background.Wait()
	return engine.Run(ctx)
}

func newDetectorRegistry(synced func(knowledge.Kind) bool) *signal.Registry {
	return signal.NewRegistry(synced,
		detect.Container{},
		detect.NewPod(detect.PodThresholds{}),
		detect.NewNode(0),
		detect.NewWorkload(0),
		detect.Job{},
		detect.HPA{},
		detect.Claim{},
		detect.Volume{},
		detect.Service{},
		detect.Certificate{},
		detect.Missing{},
	)
}

func newReasoner() *reason.Engine {
	return reason.NewEngine(0,
		reason.NodeRule{},
		reason.RolloutRule{},
		reason.ConfigRule{},
		reason.ReferenceRule{},
		reason.BackendRule{},
		reason.SchedulingRule{},
		reason.PodsRule{},
	)
}

func pruneModel(
	ctx context.Context, model *knowledge.Model, now func() time.Time,
) {
	ticker := time.NewTicker(modelPruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			model.Prune(now().Add(-modelRetention))
		}
	}
}

// coreClock adapts the application clock to the core's timer needs.
type coreClock struct {
	now interface{ Now() time.Time }
}

func (c coreClock) Now() time.Time { return c.now.Now() }

func (c coreClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
