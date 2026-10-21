package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// overlayOutcome is how the startup KwatchConfig turned out.
type overlayOutcome struct {
	// invalid is set when the object was refused.
	invalid bool
	// lastGood is set when the refused object was replaced by the last
	// spec that applied cleanly.
	lastGood bool
	// rejected identifies the refused spec, so the notice about it is sent
	// once, not at every restart.
	rejected string
	// applied is the spec (as JSON) that applied cleanly; the leader keeps
	// it as the last good one once the state file is open (see
	// rememberOverlay). Empty when there was no KwatchConfig or it was
	// refused.
	applied []byte
}

// applyStartupCRD returns the configuration kwatch runs with: the mounted
// file with the KwatchConfig overlaid. An invalid KwatchConfig is not
// fatal: a failing start would only crash-loop on the same object, so
// kwatch runs with the last spec that applied cleanly (lastGood reads it
// from the state file), or with the mounted file alone when there is none,
// and reports it. API errors still fail startup. A deleted KwatchConfig is
// a choice: the leader clears the kept spec (see rememberOverlay).
func applyStartupCRD(
	ctx context.Context,
	cfg *config.Config,
	dynamicClient dynamic.Interface,
	reload func() (*config.Config, error),
	lastGood func() ([]byte, bool),
) (effective *config.Config, outcome overlayOutcome, err error) {
	if !cfg.CrdConfig.Enabled {
		return cfg, overlayOutcome{}, nil
	}
	spec, err := crd.ApplyStartupConfigWithClient(
		ctx, cfg, dynamicClient, kubeclient.GetNamespace(),
	)
	if err == nil {
		return cfg, overlayOutcome{applied: spec}, nil
	}
	if !errors.Is(err, crd.ErrInvalidOverlay) {
		return nil, overlayOutcome{}, err
	}
	refused := err
	outcome = overlayOutcome{invalid: true, rejected: fingerprintOf(spec)}
	// The failed overlay left cfg partly applied; start from a fresh load.
	base, err := reload()
	if err != nil {
		return nil, overlayOutcome{}, err
	}
	if last, ok := lastGood(); ok {
		if crd.ApplySavedSpec(base, last) == nil {
			klog.ErrorS(refused, "KwatchConfig is invalid; starting with "+
				"the last one that applied cleanly. Fix or delete the "+
				"KwatchConfig to apply it.", "component", "crd-watcher",
				"operation", "startup", "reason", "config_overlay_invalid")
			outcome.lastGood = true
			return base, outcome, nil
		}
		// The kept spec fails today's rules: start from a fresh load.
		if base, err = reload(); err != nil {
			return nil, overlayOutcome{}, err
		}
	}
	klog.ErrorS(refused, "KwatchConfig is invalid; starting with the mounted "+
		"configuration only. Fix or delete the KwatchConfig to apply it.",
		"component", "crd-watcher", "operation", "startup",
		"reason", "config_overlay_invalid")
	return base, outcome, nil
}

// fingerprintOf names a spec by a short hash. A refusal with no spec (more
// than one object) has a fixed name.
func fingerprintOf(spec []byte) string {
	if len(spec) == 0 {
		return "no-spec"
	}
	sum := sha256.Sum256(spec)
	return hex.EncodeToString(sum[:8])
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
