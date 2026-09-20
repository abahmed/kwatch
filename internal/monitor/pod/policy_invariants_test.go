package pod

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
)

func TestDetectPodRequiresPodOnlyFindings(t *testing.T) {
	monitor := &Monitor{}

	cases := []struct {
		name       string
		pod        bool
		containers bool
		want       bool
	}{
		{name: "pod finding", pod: true, want: true},
		{name: "container finding", containers: true, want: false},
		{name: "both findings", pod: true, containers: true, want: false},
		{name: "no findings", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &enrichment.Context{
				Sources: enrichment.Sources{
					Runtime: config.RuntimeConfig{},
				},
				Findings: policy.Findings{
					PodHasIssues:        tc.pod,
					ContainersHasIssues: tc.containers,
				},
			}
			require.Equal(t, tc.want, monitor.DetectPod(ctx))
		})
	}
}

func TestBuildPodPipelineAddsOneDisruptionRule(t *testing.T) {
	disabled := false
	runtime := config.RuntimeConfigFor(&config.Config{
		IgnoreDisruptionTerminations: &disabled,
	})
	detectors := BuildPodDetectorsWithRuntimeConfig(runtime)

	count := 0
	for _, detector := range detectors {
		if _, ok := detector.(policy.DisruptionRule); ok {
			count++
		}
	}
	require.Equal(t, 0, count)

	enabled := true
	runtime = config.RuntimeConfigFor(&config.Config{
		IgnoreDisruptionTerminations: &enabled,
	})
	detectors = BuildPodDetectorsWithRuntimeConfig(runtime)
	count = 0
	for _, detector := range detectors {
		if _, ok := detector.(policy.DisruptionRule); ok {
			count++
		}
	}
	require.Equal(t, 1, count)
}
