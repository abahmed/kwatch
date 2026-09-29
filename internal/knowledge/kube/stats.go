package kube

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// StatsSource is the fact source name for kubelet statistics.
const StatsSource = "kubelet"

// Attribute names written by the stats source.
const (
	AttrFSUsedPct      = "fs.used.pct"
	AttrInodesUsedPct  = "inodes.used.pct"
	AttrMemoryPSI      = "psi.memory.some.avg60"
	AttrCPUPSI         = "psi.cpu.some.avg60"
	AttrIOPSI          = "psi.io.some.avg60"
	AttrVolumeUsedPct  = "volume.used.pct"
	AttrVolumeFillETA  = "volume.full.eta.seconds"
	AttrCPUUsageMilli  = "cpu.usage.milli"
	AttrMemoryWorking  = "memory.working.bytes"
	AttrThrottledPct   = "cpu.throttled.pct"
	AttrEphemeralUsed  = "ephemeral.used.bytes"
	AttrNetErrorRate   = "network.errors.per.second"
	AttrRuntimeErrRate = "runtime.errors.per.second"
	statsConcurrency   = 8
	statsRequestBudget = 10 * time.Second
)

// StatsConfig configures the kubelet stats poller.
type StatsConfig struct {
	Client   kubernetes.Interface
	Interval time.Duration
	Now      func() time.Time
	Submit   Submit
	// Nodes lists the nodes to poll, from the model.
	Nodes func() []knowledge.EntityID
}

// StatsPoller reads /stats/summary from every node's kubelet through the
// API server proxy and records usage as facts.
type StatsPoller struct {
	cfg      StatsConfig
	growth   *growthTracker
	counters *counterRates
}

// NewStatsPoller builds a poller.
func NewStatsPoller(cfg StatsConfig) *StatsPoller {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	return &StatsPoller{
		cfg: cfg, growth: newGrowthTracker(), counters: newCounterRates(),
	}
}

// Run polls until ctx ends.
func (p *StatsPoller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		p.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *StatsPoller) poll(ctx context.Context) {
	nodes := p.cfg.Nodes()
	slots := make(chan struct{}, statsConcurrency)
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			p.pollNode(ctx, node)
		}()
	}
	wg.Wait()
}

func (p *StatsPoller) pollNode(ctx context.Context, node knowledge.EntityID) {
	reqCtx, cancel := context.WithTimeout(ctx, statsRequestBudget)
	defer cancel()
	body, err := p.proxy(reqCtx, node, "stats/summary")
	if err != nil {
		klog.V(3).InfoS("kubelet summary unavailable", "node", node.Name,
			"error", err)
		return
	}
	var summary statsSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		return
	}
	now := p.cfg.Now()
	facts := p.facts(node, summary, now)
	facts = append(facts, p.kubeletMetrics(reqCtx, node, now)...)
	p.cfg.Submit(ctx, facts...)
}

// kubeletMetrics reads CPU throttling from cAdvisor and runtime errors
// from the kubelet's own metrics. Both are optional: failures only mean
// fewer attributes.
func (p *StatsPoller) kubeletMetrics(
	ctx context.Context, node knowledge.EntityID, now time.Time,
) []knowledge.Fact {
	var facts []knowledge.Fact
	if body, err := p.proxy(ctx, node, "metrics/cadvisor"); err == nil {
		for key, pct := range p.counters.throttleRatios(body, now) {
			parts := strings.SplitN(key, "/", 3)
			if len(parts) != 3 || parts[2] == "" {
				continue
			}
			facts = append(facts, knowledge.Fact{
				Kind: knowledge.Observed, Source: throttleSource, At: now,
				Entity: ContainerID(parts[0], parts[1], parts[2]),
				Attributes: map[string]knowledge.Value{
					AttrThrottledPct: knowledge.Number(pct),
				},
			})
		}
	}
	if body, err := p.proxy(ctx, node, "metrics"); err == nil {
		if total, ok := sumMetric(body,
			"kubelet_runtime_operations_errors_total"); ok {
			if rate, ok := p.counters.rate("runtime/"+node.Name, now,
				total); ok {
				facts = append(facts, knowledge.Fact{
					Kind: knowledge.Observed, Source: runtimeSource, At: now,
					Entity: node,
					Attributes: map[string]knowledge.Value{
						AttrRuntimeErrRate: knowledge.Number(rate),
					},
				})
			}
		}
	}
	return facts
}

func (p *StatsPoller) proxy(
	ctx context.Context, node knowledge.EntityID, path string,
) ([]byte, error) {
	return p.cfg.Client.CoreV1().RESTClient().Get().
		Resource("nodes").Name(node.Name).SubResource("proxy").
		Suffix(path).DoRaw(ctx)
}

func (p *StatsPoller) facts(
	node knowledge.EntityID, s statsSummary, now time.Time,
) []knowledge.Fact {
	attrs := map[string]knowledge.Value{}
	setPct(attrs, AttrFSUsedPct, s.Node.FS.UsedBytes, s.Node.FS.CapacityBytes)
	if s.Node.FS.Inodes > 0 {
		attrs[AttrInodesUsedPct] = knowledge.Number(
			100 * float64(s.Node.FS.Inodes-s.Node.FS.InodesFree) /
				float64(s.Node.FS.Inodes))
	}
	setPSI(attrs, AttrMemoryPSI, s.Node.Memory.PSI)
	setPSI(attrs, AttrCPUPSI, s.Node.CPU.PSI)
	setPSI(attrs, AttrIOPSI, s.Node.IO.PSI)
	if errs, ok := s.Node.Network.errors(); ok {
		if rate, ok := p.counters.rate("network/"+node.Name, now,
			float64(errs)); ok {
			attrs[AttrNetErrorRate] = knowledge.Number(rate)
		}
	}
	facts := []knowledge.Fact{{
		Kind: knowledge.Observed, Source: StatsSource, At: now,
		Entity: node, Attributes: attrs,
	}}
	facts = append(facts, podUsageFacts(s, now)...)
	for _, volume := range s.volumes() {
		claim := knowledge.NewEntityID(KindPVC, volume.Namespace, volume.Name)
		pct := percent(volume.UsedBytes, volume.CapacityBytes)
		if pct < 0 {
			continue
		}
		volumeAttrs := map[string]knowledge.Value{
			AttrVolumeUsedPct: knowledge.Number(pct),
		}
		if eta, ok := p.growth.observe(claim, now, volume.UsedBytes,
			volume.CapacityBytes); ok {
			volumeAttrs[AttrVolumeFillETA] = knowledge.Number(eta.Seconds())
		}
		facts = append(facts, knowledge.Fact{
			Kind: knowledge.Observed, Source: StatsSource, At: now,
			Entity: claim, Attributes: volumeAttrs,
		})
	}
	return facts
}

func setPct(
	attrs map[string]knowledge.Value, name string, used, capacity uint64,
) {
	if pct := percent(used, capacity); pct >= 0 {
		attrs[name] = knowledge.Number(pct)
	}
}

func percent(used, capacity uint64) float64 {
	if capacity == 0 {
		return -1
	}
	return 100 * float64(used) / float64(capacity)
}

func setPSI(attrs map[string]knowledge.Value, name string, psi *psiStats) {
	if psi != nil {
		attrs[name] = knowledge.Number(psi.Some.Avg60)
	}
}

// Sources for usage facts, kept separate so each replaces only its own
// attributes.
const (
	throttleSource = "cadvisor"
	runtimeSource  = "kubelet-metrics"
)

// podUsageFacts records container CPU and memory use and pod ephemeral
// storage use from the summary.
func podUsageFacts(s statsSummary, now time.Time) []knowledge.Fact {
	var facts []knowledge.Fact
	for _, pod := range s.Pods {
		ns, name := pod.PodRef.Namespace, pod.PodRef.Name
		if name == "" {
			continue
		}
		for _, c := range pod.Containers {
			attrs := map[string]knowledge.Value{}
			if c.CPU.UsageNanoCores != nil {
				attrs[AttrCPUUsageMilli] = knowledge.Number(
					float64(*c.CPU.UsageNanoCores) / 1e6)
			}
			if c.Memory.WorkingSetBytes != nil {
				attrs[AttrMemoryWorking] = knowledge.Number(
					float64(*c.Memory.WorkingSetBytes))
			}
			if len(attrs) > 0 {
				facts = append(facts, knowledge.Fact{
					Kind: knowledge.Observed, Source: StatsSource, At: now,
					Entity: ContainerID(ns, name, c.Name), Attributes: attrs,
				})
			}
		}
		if e := pod.EphemeralStorage; e != nil {
			facts = append(facts, knowledge.Fact{
				Kind: knowledge.Observed, Source: StatsSource, At: now,
				Entity: knowledge.NewEntityID(KindPod, ns, name),
				Attributes: map[string]knowledge.Value{
					AttrEphemeralUsed: knowledge.Number(float64(e.UsedBytes)),
				},
			})
		}
	}
	return facts
}
