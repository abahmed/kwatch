package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

// StatsSource is the observation source name for kubelet statistics.
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
	AttrMemoryRSS      = "memory.rss.bytes"
	AttrThrottledPct   = "cpu.throttled.pct"
	AttrEphemeralUsed  = "ephemeral.used.bytes"
	AttrNetErrorRate   = "network.errors.per.second"
	AttrRuntimeErrRate = "runtime.errors.per.second"
	// AttrPLEGRelistMS is the kubelet's mean pod lifecycle relist time
	// since the previous poll; a slow relist means a kubelet that falls
	// behind its pods.
	AttrPLEGRelistMS = "kubelet.pleg.relist.ms"
	// AttrEvictionRate is pods the kubelet evicted per second since the
	// previous poll.
	AttrEvictionRate   = "kubelet.evictions.per.second"
	statsConcurrency   = 8
	statsRequestBudget = 10 * time.Second
)

// Kubelet reads are bounded so one misbehaving node cannot exhaust
// memory or stall a poll round.
const (
	maxSummaryBytes = 8 << 20
	maxMetricsBytes = 16 << 20
	// sampleRounds is how many poll rounds a counter or volume sample may
	// go unseen before it is forgotten; minSampleTTL keeps short intervals
	// from dropping history during a brief node outage.
	sampleRounds = 5
	minSampleTTL = 10 * time.Minute
)

// errBodyTooLarge reports a kubelet response over its size limit.
var errBodyTooLarge = errors.New("kubelet response exceeds size limit")

// errNoKubelet reports a poller built without a kubelet client.
var errNoKubelet = errors.New("no kubelet client")

// KubeletReader opens one kubelet endpoint, such as "stats/summary", on
// a node. The composition root supplies a client that talks HTTPS to the
// kubelet directly (see internal/kubeclient.KubeletClient).
type KubeletReader interface {
	Open(ctx context.Context, node, path string) (io.ReadCloser, error)
}

// StatsConfig configures the kubelet stats poller.
type StatsConfig struct {
	Kubelet  KubeletReader
	Interval time.Duration
	Now      func() time.Time
	Submit   Submit
	// Nodes lists the nodes to poll, from the model.
	Nodes func() []inventory.EntityID
	// Containers lists the containers the model still has. The memory
	// history of a container missing from it is dropped; without it the
	// history is kept for the whole 24 hour window.
	Containers func() []inventory.EntityID
	// Report, when set, receives every round's reachability summary. It
	// runs on the poll goroutine and must not block.
	Report func(StatsRound)
}

// StatsPoller reads /stats/summary from every node's kubelet and records
// usage as observations.
type StatsPoller struct {
	cfg       StatsConfig
	growth    *growthTracker
	counters  *counterRates
	reach     *reachLog
	memory    *memoryLog
	published *publishLog
	health    selfHealthClock
	// next, failing and lastLog belong to the poll goroutine.
	next    int
	failing bool
	lastLog time.Time
}

// NewStatsPoller builds a poller.
func NewStatsPoller(cfg StatsConfig) *StatsPoller {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	return &StatsPoller{
		cfg: cfg, growth: newGrowthTracker(), counters: newCounterRates(),
		reach: newReachLog(), memory: newMemoryLog(),
		published: newPublishLog(),
	}
}

// Run polls until ctx ends.
func (p *StatsPoller) Run(ctx context.Context) {
	if p.reportNoClient(ctx) {
		<-ctx.Done()
		return
	}
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

// poll reads every node once. The whole round is bounded by the poll
// interval (at least one request budget), so a slow cluster cannot make
// rounds overlap or pile up. Every sample of a round carries the round's
// start time, so rates compare whole rounds. Each round starts where the
// previous one stopped, so a slow cluster spreads the gaps across nodes.
func (p *StatsPoller) poll(ctx context.Context) {
	now := p.cfg.Now()
	roundCtx, cancel := context.WithTimeout(ctx,
		max(p.cfg.Interval, statsRequestBudget))
	defer cancel()
	slots := make(chan struct{}, statsConcurrency)
	var wg sync.WaitGroup
	tally := &statsTally{}
	nodes := p.rotate(p.cfg.Nodes())
	started := 0
	for _, node := range nodes {
		if roundCtx.Err() != nil {
			break
		}
		started++
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			err := p.pollNode(ctx, roundCtx, node, now)
			tally.add(node, err)
			p.recordReach(ctx, node, now, err)
		}()
	}
	wg.Wait()
	p.advance(len(nodes), started)
	p.clearDeparted(ctx, nodes, now)
	p.forgetStale(ctx, now)
	if ctx.Err() == nil {
		p.finishRound(now, tally, tally.round(len(nodes), started))
	}
}

// forgetStale drops rate and growth samples for containers, nodes and
// volumes that no poll has reported for several rounds.
func (p *StatsPoller) forgetStale(ctx context.Context, now time.Time) {
	cutoff := now.Add(-max(sampleRounds*p.cfg.Interval, minSampleTTL))
	p.counters.forgetBefore(cutoff)
	p.growth.forgetBefore(cutoff)
	p.reach.forgetBefore(now.Add(-KubeletFailureWindow))
	p.clearMemory(ctx, now)
	p.logSelfHealth(now)
}

// pollNode reads one node within roundCtx and submits with ctx, so a
// result that arrived in time is not lost when the round deadline passes.
// It returns why the summary could not be read; optional metrics endpoints
// never fail a node.
func (p *StatsPoller) pollNode(
	ctx, roundCtx context.Context, node inventory.EntityID, now time.Time,
) error {
	reqCtx, cancel := context.WithTimeout(roundCtx, statsRequestBudget)
	defer cancel()
	body, err := p.read(reqCtx, node, "stats/summary", maxSummaryBytes)
	if err != nil {
		klog.V(3).InfoS("kubelet summary unavailable", "node", node.Name,
			"error", err)
		p.clearAfterFailure(ctx, node, now)
		return err
	}
	var summary statsSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		p.clearAfterFailure(ctx, node, now)
		return fmt.Errorf("decode kubelet summary: %w", err)
	}
	observations := p.observations(node, summary, now)
	observations = append(observations, p.kubeletMetrics(reqCtx, node, now)...)
	observations = append(observations,
		p.published.answered(node.Name, observations, now)...)
	observations = append(observations,
		p.memory.observations(node.Name, now)...)
	p.cfg.Submit(ctx, observations...)
	return nil
}

func (p *StatsPoller) observations(
	node inventory.EntityID, s statsSummary, now time.Time,
) []inventory.Observation {
	attrs := map[string]inventory.Value{}
	setPct(attrs, AttrFSUsedPct, s.Node.FS.UsedBytes, s.Node.FS.CapacityBytes)
	if fs := s.Node.FS; fs.Inodes > 0 && fs.InodesFree <= fs.Inodes {
		// A free count above the total is a bad reading, not a negative
		// use (the unsigned subtraction would wrap around).
		attrs[AttrInodesUsedPct] = inventory.Number(
			100 * float64(fs.Inodes-fs.InodesFree) / float64(fs.Inodes))
	}
	setPSI(attrs, AttrMemoryPSI, s.Node.Memory.PSI)
	setPSI(attrs, AttrCPUPSI, s.Node.CPU.PSI)
	setPSI(attrs, AttrIOPSI, s.Node.IO.PSI)
	if errs, ok := s.Node.Network.errors(); ok {
		if rate, ok := p.counters.rate("network/"+node.Name, now,
			float64(errs)); ok {
			attrs[AttrNetErrorRate] = inventory.Number(rate)
		}
	}
	observations := []inventory.Observation{{
		Kind: inventory.Observed, Source: StatsSource, At: now,
		Entity: node, Attributes: attrs,
	}}
	observations = append(observations,
		p.podUsageObservations(node.Name, s, now)...)
	for _, volume := range s.volumes() {
		claim := inventory.CoreID(KindPVC, volume.Namespace, volume.Name)
		pct := percent(volume.UsedBytes, volume.CapacityBytes)
		if pct < 0 {
			continue
		}
		volumeAttrs := map[string]inventory.Value{
			AttrVolumeUsedPct: inventory.Number(pct),
		}
		if eta, ok := p.growth.observe(claim, now, volume.UsedBytes,
			volume.CapacityBytes); ok {
			volumeAttrs[AttrVolumeFillETA] = inventory.Number(eta.Seconds())
		}
		observations = append(observations, inventory.Observation{
			Kind: inventory.Observed, Source: StatsSource, At: now,
			Entity: claim, Attributes: volumeAttrs,
		})
	}
	return observations
}

func setPct(
	attrs map[string]inventory.Value, name string, used, capacity uint64,
) {
	if pct := percent(used, capacity); pct >= 0 {
		attrs[name] = inventory.Number(pct)
	}
}

func percent(used, capacity uint64) float64 {
	if capacity == 0 {
		return -1
	}
	return 100 * float64(used) / float64(capacity)
}

func setPSI(attrs map[string]inventory.Value, name string, psi *psiStats) {
	if psi != nil {
		attrs[name] = inventory.Number(psi.Some.Avg60)
	}
}

// Sources for usage observations, kept separate so each replaces only its own
// attributes.
const (
	throttleSource = "cadvisor"
	runtimeSource  = "kubelet-metrics"
	// reachSource carries the kubelet reach count of a node, apart from
	// the usage readings, which are replaced as a whole on every poll.
	reachSource = "kubelet-reach"
	// memorySource carries the memory history of a container. It outlives
	// the container's presence in the kubelet summary.
	memorySource = "kubelet-memory"
)

// EnrichmentSources are the observation sources of the stats poller, the
// automatic Service prober and the crash-log round. They only add data to
// entities the informers observe, so the model must not let them create an
// entity or resurrect a deleted one (see inventory.Options.EnrichmentSources).
func EnrichmentSources() []string {
	return []string{
		StatsSource, throttleSource, runtimeSource, reachSource,
		memorySource, serviceProbeSource, crashLogSource,
		webhookMetricsSource,
	}
}
