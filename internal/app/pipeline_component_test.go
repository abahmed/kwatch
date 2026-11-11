package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The pipeline reports nothing while it builds, restores and runs its
// first pass, so its component must allow what liveness allows and
// ignore the progress stamped before this run began.
func TestPipelineComponentStartupWiring(t *testing.T) {
	deps := &serverDeps{
		pipelineProgress: newComponentProgress(time.Now()),
	}
	spec := pipelineComponent(deps, activeResources{})

	require.Equal(t, "pipeline", spec.name)
	require.True(t, spec.required)
	require.Same(t, deps.pipelineProgress, spec.progress)
	require.Equal(t, pipelineLivenessStall, spec.startupTimeout,
		"the startup deadline must match what liveness allows")
	require.True(t, spec.freshStart,
		"an older progress stamp must not end the startup wait")
}
