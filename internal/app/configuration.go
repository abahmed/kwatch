package app

import (
	"context"

	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/crdwatch"
	"github.com/abahmed/kwatch/internal/k8s"
)

func loadConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	// Legal but self-defeating combinations: loud once at startup beats
	// notifications that are quietly wrong for the life of the process.
	for _, warning := range config.Warnings(cfg) {
		klog.InfoS("configuration warning", "detail", warning)
	}
	return cfg, nil
}

func applyStartupCRD(
	ctx context.Context,
	cfg *config.Config,
	dynamicClient dynamic.Interface,
) error {
	if !cfg.CrdConfig.Enabled {
		return nil
	}
	return crdwatch.ApplyStartupConfigWithClient(
		ctx, cfg, dynamicClient, k8s.GetNamespace(),
	)
}
