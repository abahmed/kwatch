package crdwatch

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/config"
)

// New is retained for callers that still own a REST configuration. The
// application uses NewWithClient so dynamic client construction is centralized.
func New(
	cfg *config.Config,
	restConfig *rest.Config,
	namespace string,
	resync time.Duration,
	restart func(),
) *Watcher {
	return &Watcher{
		runtime: config.RuntimeConfigFor(cfg),
		legacyClient: func() (dynamic.Interface, error) {
			client, err := dynamic.NewForConfig(restConfig)
			if err != nil {
				return nil, fmt.Errorf(
					"crdwatch: failed to create dynamic client: %w", err,
				)
			}
			return client, nil
		},
		namespace: namespace,
		resync:    resync,
		seen:      make(map[string]string),
		restart:   restart,
	}
}

// ApplyStartupConfig overlays the optional KwatchConfig using a legacy REST
// configuration. New application code uses ApplyStartupConfigWithClient.
func ApplyStartupConfig(
	ctx context.Context,
	cfg *config.Config,
	restConfig *rest.Config,
	namespace string,
) error {
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	return applyStartupConfig(ctx, cfg, dynamicClient, namespace)
}
