package main

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

func runLint(strict, check bool, out, errOut io.Writer) int {
	cfg, err := config.LoadConfig()
	if err != nil {
		if _, writeErr := fmt.Fprintf(errOut, "ERROR: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	errs := config.ValidateConfig(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			if _, writeErr := fmt.Fprintf(errOut, "  %s\n", e); writeErr != nil {
				return 1
			}
		}
		return 1
	}
	// Warnings describe a configuration that works but will mislead. They are
	// reported, never fatal: a lint that fails on a suboptimal setting is a
	// lint people stop running.
	for _, warning := range config.LintWarnings(cfg) {
		if _, writeErr := fmt.Fprintf(
			out, "  warning: %s\n", warning,
		); writeErr != nil {
			return 1
		}
	}
	if strict {
		if err := config.LintStrict(); err != nil {
			if _, writeErr := fmt.Fprintf(
				errOut, "STRICT ERROR: %v\n", err,
			); writeErr != nil {
				return 1
			}
			return 1
		}
	}
	// Construction only validates settings and builds clients; it does no
	// I/O, so it always runs. Only --check contacts providers.
	am, code := constructProviders(cfg, errOut)
	if code != 0 {
		return 1
	}
	if check {
		if verifyProviders(am, out, errOut) != 0 {
			return 1
		}
	}
	if _, err := fmt.Fprintln(out, "config OK"); err != nil {
		return 1
	}
	return 0
}

// constructProviders builds the configured providers exactly as startup
// does, so a setting a constructor refuses fails lint too.
func constructProviders(
	cfg *config.Config, errOut io.Writer,
) (*delivery.Manager, int) {
	runtime := config.RuntimeConfigFor(cfg)
	am := delivery.NewManagerWithDependencies(delivery.Dependencies{
		HTTPClient: kubeclient.NewHTTPClientWithRuntime(runtime),
		Clock:      clock.RealClock{},
	})
	if err := am.InitRuntime(runtime, catalog.NewProvider); err != nil {
		if _, writeErr := fmt.Fprintf(
			errOut, "ERROR: initialize providers: %v\n", err,
		); writeErr != nil {
			return nil, 1
		}
		return nil, 1
	}
	return am, 0
}

func verifyProviders(am *delivery.Manager, out, errOut io.Writer) int {
	results := am.VerifyAll(context.Background())
	hasErr := false
	names := make([]string, 0, len(results))
	for name := range results {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		err := results[name]
		if err != nil {
			if _, writeErr := fmt.Fprintf(
				errOut, "  %s: FAIL — %v\n", name, err,
			); writeErr != nil {
				return 1
			}
			hasErr = true
			continue
		}
		if _, writeErr := fmt.Fprintf(out, "  %s: OK\n", name); writeErr != nil {
			return 1
		}
	}
	if hasErr {
		return 1
	}
	return 0
}
