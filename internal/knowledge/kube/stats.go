package kube

import (
	"context"
	"encoding/json"
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
	cfg    StatsConfig
	growth *growthTracker
}

// NewStatsPoller builds a poller.
func NewStatsPoller(cfg StatsConfig) *StatsPoller {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	return &StatsPoller{cfg: cfg, growth: newGrowthTracker()}
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
	body, err := p.cfg.Client.CoreV1().RESTClient().Get().
		Resource("nodes").Name(node.Name).SubResource("proxy").
		Suffix("stats/summary").DoRaw(reqCtx)
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
	p.cfg.Submit(ctx, p.facts(node, summary, now)...)
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
	facts := []knowledge.Fact{{
		Kind: knowledge.Observed, Source: StatsSource, At: now,
		Entity: node, Attributes: attrs,
	}}
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
