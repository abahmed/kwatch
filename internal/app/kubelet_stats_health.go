package app

import (
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
)

// kubeletStatsComponent is the health component for kubelet reachability.
// Kubelet stats are optional enrichment: losing them degrades /health but
// never readiness.
const kubeletStatsComponent = "kubelet-stats"

// kubeletStatsHealth turns stats rounds into a health component and the
// kwatch_kubelet_stats_failures_total counter.
type kubeletStatsHealth struct {
	sink     sourceStatusSink
	registry *metrics.Registry
}

func newKubeletStatsHealth(
	sink sourceStatusSink, registry *metrics.Registry,
) *kubeletStatsHealth {
	return &kubeletStatsHealth{sink: sink, registry: registry}
}

// report is the poller's Report callback. The health server records a
// transition only when state or reason changes, so calling it every round
// is cheap.
func (h *kubeletStatsHealth) report(round kube.StatsRound) {
	if h.registry != nil && round.Failed > 0 {
		h.registry.KubeletStatsFailures.Add(int64(round.Failed))
	}
	if h.sink == nil {
		return
	}
	if round.Reason == "" {
		h.sink.SetComponentStatus(
			kubeletStatsComponent, "running", "", true)
		return
	}
	h.sink.SetComponentStatus(
		kubeletStatsComponent, "degraded", round.Reason, false)
}

// healthSink is the health server as a status sink, or nil when health is
// not served (a nil pointer must not become a non-nil interface).
func healthSink(deps *serverDeps) sourceStatusSink {
	if deps.healthServer == nil {
		return nil
	}
	return deps.healthServer
}
