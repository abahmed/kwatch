package app

import (
	"context"
	"errors"

	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/config/crd"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

func loadConfig() (*config.Config, error) {
	return config.LoadConfig()
}

// logConfigWarnings reports legal but self-defeating combinations and
// unknown keys. Loud once at startup beats notifications that are quietly
// wrong for the life of the process.
func logConfigWarnings(cfg *config.Config) {
	for _, warning := range config.Warnings(cfg) {
		klog.InfoS("configuration warning", "detail", warning)
	}
}

// overlayHealthComponent names the /health entry for a KwatchConfig that
// kwatch refused to apply.
const overlayHealthComponent = "config-overlay"

// applyStartupCRD returns the configuration kwatch runs with: the mounted
// file with the KwatchConfig overlaid. An invalid KwatchConfig is not
// fatal: a failing start would only crash-loop on the same object, so
// kwatch reloads the mounted file, runs with it alone and reports
// overlayInvalid. API errors still fail startup.
func applyStartupCRD(
	ctx context.Context,
	cfg *config.Config,
	dynamicClient dynamic.Interface,
	reload func() (*config.Config, error),
) (effective *config.Config, overlayInvalid bool, err error) {
	if !cfg.CrdConfig.Enabled {
		return cfg, false, nil
	}
	err = crd.ApplyStartupConfigWithClient(
		ctx, cfg, dynamicClient, kubeclient.GetNamespace(),
	)
	if err == nil {
		return cfg, false, nil
	}
	if !errors.Is(err, crd.ErrInvalidOverlay) {
		return nil, false, err
	}
	klog.ErrorS(err, "KwatchConfig is invalid; starting with the mounted "+
		"configuration only. Fix or delete the KwatchConfig to apply it.",
		"component", "crd-watcher", "operation", "startup",
		"reason", "config_overlay_invalid")
	// The failed overlay left cfg partly applied; start from a fresh load.
	base, err := reload()
	if err != nil {
		return nil, false, err
	}
	return base, true, nil
}

// reportOverlayHealth publishes a rejected startup KwatchConfig on /health.
// It is a degradation, not a readiness failure: monitoring runs with the
// mounted configuration.
func reportOverlayHealth(server *health.HealthServer, overlayInvalid bool) {
	if !overlayInvalid || server == nil {
		return
	}
	server.SetComponentStatus(overlayHealthComponent, "degraded",
		"config_overlay_invalid", false)
}
