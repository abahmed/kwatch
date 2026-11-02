package dashboard

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// usageSource is the kubelet stats poller's hourly peaks of one
// container over the last 24 hours, oldest first.
type usageSource func(id inventory.EntityID) []kube.UsageHour

// Usage history bounds: the containers drawn and the hours of each.
const (
	maxUsageHistories = 3
	maxUsagePeaks     = 24
)

// Metrics of a usage history.
const (
	metricMemory = "memory"
	metricCPU    = "cpu"
)

// nearLimit is the share of its memory limit a container uses from
// which its memory is evidence even when no finding is about memory.
const nearLimit = 0.9

// SetUsageHours sets where the incident page reads the containers'
// hourly usage peaks; nil removes it (the stats poller does not run).
func (s *Server) SetUsageHours(
	src func(inventory.EntityID) []kube.UsageHour,
) {
	if src == nil {
		s.sources.usage.Store(nil)
		return
	}
	v := usageSource(src)
	s.sources.usage.Store(&v)
}

// usageOf is the usage history of p's containers when it is evidence:
// memory when a finding is about memory or a container was killed for
// it or runs near its limit, CPU when a finding is CPU throttling.
// Other incidents get none. cut counts the histories left out.
func usageOf(
	p incident.Incident, rows []ContainerRow, src usageSource,
) ([]UsageHistory, int) {
	if src == nil || len(rows) == 0 {
		return nil, 0
	}
	memory, cpu := usageEvidence(p)
	var out []UsageHistory
	add := func(r ContainerRow, metric string, limit int64) {
		if h, ok := usageHistory(r, metric, limit, src); ok {
			out = append(out, h)
		}
	}
	for _, r := range rows {
		if memory || memoryHot(r) {
			add(r, metricMemory, r.MemoryLimit)
		}
		if cpu {
			add(r, metricCPU, r.CPULimit)
		}
	}
	if len(out) > maxUsageHistories {
		return out[:maxUsageHistories], len(out) - maxUsageHistories
	}
	return out, 0
}

// usageEvidence reports whether p's failure findings are about memory
// and about CPU throttling.
func usageEvidence(p incident.Incident) (memory, cpu bool) {
	memory = memoryMode(p.Mode)
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		switch f.Reason {
		case reasons.OOMKilled, reasons.OOMKILLED,
			reasons.ContainerMemoryHigh:
			memory = true
		case reasons.ContainerCPUThrottled:
			cpu = true
		}
		memory = memory || memoryMode(f.Mode)
	}
	return memory, cpu
}

func memoryMode(m detection.Mode) bool {
	return strings.HasPrefix(string(m), string(detection.ModeOOMKilled)) ||
		m == detection.ModeMemory
}

// memoryHot reports a container killed for memory or near its limit.
func memoryHot(r ContainerRow) bool {
	if r.LastReason == reasons.OOMKilled ||
		r.StateReason == reasons.OOMKilled {
		return true
	}
	return r.MemoryLimit > 0 &&
		float64(r.MemoryUsed) >= nearLimit*float64(r.MemoryLimit)
}

// usageHistory reads r's peaks of metric; false without any.
func usageHistory(
	r ContainerRow, metric string, limit int64, src usageSource,
) (UsageHistory, bool) {
	hours := src(kube.ContainerID(r.Namespace, r.Pod, r.Container))
	if len(hours) > maxUsagePeaks {
		hours = hours[len(hours)-maxUsagePeaks:]
	}
	h := UsageHistory{Namespace: r.Namespace, Pod: r.Pod,
		Container: r.Container, Metric: metric, Limit: limit}
	seen := false
	for _, u := range hours {
		v := u.MemoryBytes
		if metric == metricCPU {
			v = u.CPUMilli
		}
		seen = seen || v > 0
		h.Peaks = append(h.Peaks, UsagePeak{Start: u.Start,
			Value: int64(v)})
	}
	return h, seen
}
