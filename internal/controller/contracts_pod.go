package controller

import (
	"context"

	"github.com/abahmed/kwatch/internal/monitor/pod"
)

// PodProcessor handles Pod queue items.
type PodProcessor interface {
	ProcessPod(context.Context, string, bool) error
}

// PodConfig wires synchronized Pod sources into the Pod runtime.
type PodConfig interface {
	ConfigureSources(pod.RuntimeSources) error
}
