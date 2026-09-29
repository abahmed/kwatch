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
	cfg.Runtime = config.CompileRuntimeConfig(cfg)

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	boot, err := newBootstrap(ctx, cfg, now)
	if err != nil {
		klog.ErrorS(err, "failed to initialize application")
		return 1
	}
	deps := newServerDeps(ctx, cancel, boot)
	if err := openHealth(deps); err != nil {
		klog.ErrorS(err, "failed to open health server")
		return 1
	}
	return serve(ctx, deps)
}
