package app

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNoProvidersNoticeWarnsWhenNothingWouldReceiveAnAlert(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"defaults":         config.DefaultConfig(),
		"file, no alert":   {Namespaces: []string{"apps"}},
		"empty alert map":  {Alert: map[string]map[string]interface{}{}},
		"removed provider": {Alert: map[string]map[string]interface{}{"line": {}}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg.Runtime = config.CompileRuntimeConfig(cfg)
			require.Contains(t, noProvidersNotice(cfg.Runtime),
				"no alert provider is configured")
		})
	}
}

func TestNoProvidersNoticeIsEmptyWithAProvider(t *testing.T) {
	cfg := deliveryConfig()
	cfg.Runtime = config.CompileRuntimeConfig(cfg)
	require.Empty(t, noProvidersNotice(cfg.Runtime))
}

func TestLogNoProvidersWritesAWarning(t *testing.T) {
	out := captureKlog(t)
	cfg := config.DefaultConfig()
	cfg.Runtime = config.CompileRuntimeConfig(cfg)

	logNoProviders(cfg.Runtime)
	klog.Flush()

	require.Equal(t, 1, distinctLines(out.String(), "no alert provider"))
	require.Contains(t, out.String(), "W")
}
