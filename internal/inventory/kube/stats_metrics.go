package kube

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// kubeletMetrics reads CPU throttling from cAdvisor and runtime errors
// from the kubelet's own metrics. Both are optional: failures only mean
// fewer attributes.
func (p *StatsPoller) kubeletMetrics(
	ctx context.Context, node inventory.EntityID, now time.Time,
) []inventory.Observation {
	var observations []inventory.Observation
	if body, err := p.read(ctx, node, "metrics/cadvisor",
		maxMetricsBytes); err == nil {
		for key, pct := range p.counters.throttleRatios(body, now) {
			parts := strings.SplitN(key, "/", 3)
			if len(parts) != 3 || parts[2] == "" {
				continue
			}
			observations = append(observations, inventory.Observation{
				Kind: inventory.Observed, Source: throttleSource, At: now,
				Entity: ContainerID(parts[0], parts[1], parts[2]),
				Attributes: map[string]inventory.Value{
					AttrThrottledPct: inventory.Number(pct),
				},
			})
		}
	}
	if body, err := p.read(ctx, node, "metrics", maxMetricsBytes); err == nil {
		if attrs := p.kubeletHealth(node, body, now); len(attrs) > 0 {
			observations = append(observations, inventory.Observation{
				Kind: inventory.Observed, Source: runtimeSource, At: now,
				Entity: node, Attributes: attrs,
			})
		}
	}
	return observations
}

// kubeletHealth reads the kubelet's own counters into rates since the
// previous poll: runtime errors, pod lifecycle relist time, evictions.
func (p *StatsPoller) kubeletHealth(
	node inventory.EntityID, body []byte, now time.Time,
) map[string]inventory.Value {
	attrs := map[string]inventory.Value{}
	if total, ok := sumMetric(body,
		"kubelet_runtime_operations_errors_total"); ok {
		if rate, ok := p.counters.rate("runtime/"+node.Name, now,
			total); ok {
			attrs[AttrRuntimeErrRate] = inventory.Number(rate)
		}
	}
	if sum, ok1 := sumMetric(body,
		"kubelet_pleg_relist_duration_seconds_sum"); ok1 {
		if count, ok2 := sumMetric(body,
			"kubelet_pleg_relist_duration_seconds_count"); ok2 {
			if mean, ok := p.counters.meanRate("pleg/"+node.Name, now,
				sum, count); ok {
				attrs[AttrPLEGRelistMS] = inventory.Number(mean * 1000)
			}
		}
	}
	if total, ok := sumMetric(body, "kubelet_evictions"); ok {
		if rate, ok := p.counters.rate("evictions/"+node.Name, now,
			total); ok {
			attrs[AttrEvictionRate] = inventory.Number(rate)
		}
	}
	return attrs
}

// read reads one kubelet endpoint, refusing a body larger than limit
// bytes.
func (p *StatsPoller) read(
	ctx context.Context, node inventory.EntityID, path string, limit int64,
) ([]byte, error) {
	if p.cfg.Kubelet == nil {
		return nil, errNoKubelet
	}
	stream, err := p.cfg.Kubelet.Open(ctx, node.Name, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	body, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: %s over %d bytes",
			errBodyTooLarge, path, limit)
	}
	return body, nil
}
