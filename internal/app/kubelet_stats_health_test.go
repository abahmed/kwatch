package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestKubeletStatsHealthPublishesReachability(t *testing.T) {
	sink := &fakeStatusSink{}
	registry := &metrics.Registry{}
	health := newKubeletStatsHealth(sink, registry)

	health.report(kube.StatsRound{
		Nodes: 3, Failed: 2, Reason: kube.StatsReasonPartial,
	})
	require.Equal(t, recordedStatus{
		"degraded", kube.StatsReasonPartial, false,
	}, sink.get(kubeletStatsComponent))
	require.EqualValues(t, 2, registry.KubeletStatsFailures.Load())

	health.report(kube.StatsRound{Nodes: 3})
	require.Equal(t, recordedStatus{"running", "", true},
		sink.get(kubeletStatsComponent))
	require.EqualValues(t, 2, registry.KubeletStatsFailures.Load())
}

func TestKubeletStatsHealthWithoutHealthServer(t *testing.T) {
	registry := &metrics.Registry{}
	health := newKubeletStatsHealth(healthSink(&serverDeps{}), registry)

	health.report(kube.StatsRound{
		Nodes: 1, Failed: 1, Reason: kube.StatsReasonUnreachable,
	})

	require.EqualValues(t, 1, registry.KubeletStatsFailures.Load())
}
