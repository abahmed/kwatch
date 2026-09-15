package controller

import (
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

// New is retained for embedded callers that still provide YAML configuration.
// Application composition uses NewWithRuntimeConfig.
func New(
	client kubernetes.Interface,
	cfg *config.Config,
	components RuntimeSet,
	clocks ...func() time.Time,
) (*Controller, func(), error) {
	runtime := config.RuntimeConfigFor(cfg)
	return NewWithRuntimeConfig(client, runtime, components, clock.From(clocks))
}
