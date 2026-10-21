package app

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/storage"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

// genHarness drives the generation loop without a cluster. Hooks are
// indexed by generation, counted from one.
type genHarness struct {
	t      *testing.T
	sleeps *sleepRecorder
	gen    int
	events []string
	boots  int

	loadErr func(gen int) error
	bootErr func(gen, attempt int) error
	health  func(gen int) config.HealthCheck
	serve   func(gen int, deps *serverDeps) int
	// tweakBoot changes the bootstrap of every generation.
	tweakBoot func(*bootstrap)
	// fired makes every release wait give up at once.
	fired bool
}

func newGenHarness(t *testing.T) *genHarness {
	t.Helper()
	return &genHarness{t: t, sleeps: &sleepRecorder{}}
}

func (h *genHarness) env() generationEnv {
	now := time.Now
	return generationEnv{
		now: now, processStart: now(), retry: h.sleeps.retry(),
		after: h.after, load: h.load, boot: h.boot, serve: h.run,
		logs: newLogFormat(),
	}
}

func (h *genHarness) after(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	if h.fired {
		ch <- time.Time{}
	}
	return ch
}

func (h *genHarness) load() (*config.Config, error) {
	h.gen++
	h.boots = 0
	h.events = append(h.events, "load")
	if h.loadErr != nil {
		if err := h.loadErr(h.gen); err != nil {
			return nil, err
		}
	}
	cfg := &config.Config{}
	if h.health != nil {
		cfg.HealthCheck = h.health(h.gen)
	}
	cfg.Runtime = config.CompileRuntimeConfig(cfg)
	return cfg, nil
}

func (h *genHarness) boot(
	_ context.Context, cfg *config.Config, _ func() time.Time, _ time.Time,
	entitlements feature.Entitlements,
) (*bootstrap, error) {
	h.boots++
	h.events = append(h.events, "boot")
	if h.bootErr != nil {
		if err := h.bootErr(h.gen, h.boots); err != nil {
			return nil, err
		}
	}
	clk := clock.Func(time.Now)
	runtime := cfg.Runtime
	boot := &bootstrap{
		runtime: runtime, clients: kubeclient.ClientSet{Clock: clk},
		healthServer: newHealthServer(runtime, clk, false, entitlements),
		deliveryManager: delivery.NewManagerWithDependencies(
			delivery.Dependencies{Clock: clk}),
		clock: clk, threadWake: &threadWake{}, entitlements: entitlements,
	}
	if h.tweakBoot != nil {
		h.tweakBoot(boot)
	}
	return boot, nil
}

func (h *genHarness) run(_ context.Context, deps *serverDeps) int {
	h.events = append(h.events, "serve")
	defer func() { h.events = append(h.events, "served") }()
	if h.serve == nil {
		return endGeneration(deps, nil)
	}
	return h.serve(h.gen, deps)
}

// endGeneration ends a generation the way the production shutdown does:
// the health server runs under the supervisor and waitShutdown stops
// everything. A non-nil next asks for an edition rebuild.
func endGeneration(deps *serverDeps, next *feature.Entitlements) int {
	supervisor := newComponentSupervisor(time.Now)
	if deps.runtime.Lifecycle().HealthCheck().Enabled {
		supervisor.startOwned(deps.ctx, componentSpec{
			name: "health-server", required: true,
			run: deps.healthServer.Serve,
		})
	}
	if next == nil {
		deps.cancel()
	} else {
		deps.editionRestart.Store(true)
		deps.edition.Store(*next)
		supervisor.errCh <- errEditionChanged
	}
	return waitShutdown(deps, supervisor)
}

func restartTo(e feature.Entitlements) *feature.Entitlements { return &e }

// holdStateFile opens the state file the way a leader session does and
// records it on deps, so the loop can wait for it.
func holdStateFile(
	t *testing.T, deps *serverDeps,
) (*storage.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := storage.Open(path, storage.Options{Now: time.Now})
	require.NoError(t, err)
	_, err = s.Claim()
	require.NoError(t, err)
	deps.openedStore.Store(s)
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

// freeHolder takes the port once generation one has released it (the
// rebuild keeps it open until the next generation is built, so the
// release is forced here) and keeps it until the returned function runs.
func freeHolder(t *testing.T, port int, h *genHarness) func() {
	t.Helper()
	var held net.Listener
	inner := h.serve
	h.serve = func(gen int, deps *serverDeps) int {
		code := inner(gen, deps)
		if gen == 1 {
			stopHealthServer(deps)
			ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
			require.NoError(t, err)
			held = ln
		}
		return code
	}
	return func() {
		if held != nil {
			_ = held.Close()
		}
	}
}
