package kube

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// podUsageObservations records container CPU and memory use and pod ephemeral
// storage use from the summary.
func (p *StatsPoller) podUsageObservations(
	node string, s statsSummary, now time.Time,
) []inventory.Observation {
	var observations []inventory.Observation
	for _, pod := range s.Pods {
		ns, name := pod.PodRef.Namespace, pod.PodRef.Name
		if name == "" {
			continue
		}
		for _, c := range pod.Containers {
			if o, ok := p.containerUsage(node, ns, name, c, now); ok {
				observations = append(observations, o)
			}
		}
		if e := pod.EphemeralStorage; e != nil {
			observations = append(observations, inventory.Observation{
				Kind: inventory.Observed, Source: StatsSource, At: now,
				Entity: inventory.CoreID(KindPod, ns, name),
				Attributes: map[string]inventory.Value{
					AttrEphemeralUsed: inventory.Number(float64(e.UsedBytes)),
				},
			})
		}
	}
	return observations
}

// containerUsage records one container's CPU and memory, if reported,
// and notes the reading in the memory history.
func (p *StatsPoller) containerUsage(
	node, ns, pod string, c statsContainer, now time.Time,
) (inventory.Observation, bool) {
	attrs := map[string]inventory.Value{}
	id := ContainerID(ns, pod, c.Name)
	if cpu := c.CPU.UsageNanoCores; cpu != nil {
		attrs[AttrCPUUsageMilli] = inventory.Number(
			float64(*cpu) / 1e6)
	}
	if memory := c.Memory.WorkingSetBytes; memory != nil {
		attrs[AttrMemoryWorking] = inventory.Number(
			float64(*memory))
		p.memory.note(id, node, c.startTime(), now, float64(*memory))
	}
	if rss := c.Memory.RSSBytes; rss != nil {
		attrs[AttrMemoryRSS] = inventory.Number(float64(*rss))
	}
	if len(attrs) == 0 {
		return inventory.Observation{}, false
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: StatsSource, At: now,
		Entity: id, Attributes: attrs,
	}, true
}
