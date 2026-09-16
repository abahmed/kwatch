package app

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/version"
)

// Run loads config and wires all monitors, then runs until shutdown.
func Run() int {
	return RunWithClock(clock.RealClock{}.Now)
}

// RunWithClock runs kwatch with an injected clock, primarily for deterministic
// integration tests and embedded callers.
func RunWithClock(now func() time.Time) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := loadConfig()
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		return 1
	}
	if _, err := newMonitorRegistry(); err != nil {
		klog.ErrorS(err, "invalid monitor registry")
		return 1
	}
	cfg.WatchStartTime = now()
	// WatchStartTime is derived at process start, so compile the snapshot only
	// after it has been stamped. Runtime consumers must see one consistent
	// startup boundary.
	cfg.Runtime = config.CompileRuntimeConfig(cfg)

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	boot, err := newBootstrap(ctx, cfg, now)
	if err != nil {
		klog.ErrorS(err, "failed to initialize application")
		return 1
	}
	deps, err := buildServerDeps(
		ctx, cancel, boot.runtime, boot, now,
	)
	if err != nil {
		klog.ErrorS(err, "failed to build application runtime")
		return 1
	}
	return serve(ctx, deps)
}
