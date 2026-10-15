package app

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/version"
)

// Run loads config and wires all monitors, then runs until shutdown.
func Run() int {
	return RunWithClock(clock.RealClock{}.Now)
}

// RunWithClock runs kwatch with an injected clock, primarily for deterministic
// integration tests and embedded callers.
//
// The edition is not fixed for the life of the process: the leader's
// license manager decides it. When a license starts or stops entitling Pro,
// the leader session ends and kwatch rebuilds in process, so every
// component is built with the new entitlements and the pod never restarts.
func RunWithClock(now func() time.Time) int {
	entitlements := ResolveEntitlements()
	for {
		code, next, again := runOnce(now, entitlements)
		if !again {
			return code
		}
		entitlements = next
	}
}

// runOnce builds and serves one generation of the application. It reports
// the entitlements to rebuild with when the license changed the edition.
func runOnce(
	now func() time.Time, entitlements feature.Entitlements,
) (int, feature.Entitlements, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := loadConfig()
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		return 1, nil, false
	}
	applyLogFormat(cfg.Runtime.Application().LogFormatter)

	klog.InfoS(fmt.Sprintf(welcomeMessage, version.Short()))

	boot, err := newBootstrap(ctx, cfg, now, entitlements)
	if err != nil {
		klog.ErrorS(err, "failed to initialize application")
		return 1, nil, false
	}
	// The KwatchConfig overlay may change app.logFormatter.
	applyLogFormat(boot.runtime.Application().LogFormatter)
	logSecretsAccess(boot.runtime)
	logInactivePro(entitlements, boot.inactivePro)
	deps := newServerDeps(ctx, cancel, boot)
	if err := deps.healthServer.Open(); err != nil {
		klog.ErrorS(err, "failed to open health server")
		return 1, nil, false
	}
	code := serve(ctx, deps)
	if deps.editionRestart.Load() {
		return code, deps.edition.Load(), true
	}
	return code, nil, false
}
