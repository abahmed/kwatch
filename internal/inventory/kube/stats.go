package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
	// Report, when set, receives every round's reachability summary. It
	// runs on the poll goroutine and must not block.
	Report func(StatsRound)
}

// StatsPoller reads /stats/summary from every node's kubelet and records
// usage as observations.
type StatsPoller struct {
	cfg      StatsConfig
	growth   *growthTracker
	counters *counterRates
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
			tally.add(node, p.pollNode(ctx, roundCtx, node, now))
		}()
	}
	wg.Wait()
	p.advance(len(nodes), started)
	p.forgetStale(now)
	if ctx.Err() == nil {
		p.finishRound(now, tally, tally.round(len(nodes), started))
	}
}

// forgetStale drops rate and growth samples for containers, nodes and
// volumes that no poll has reported for several rounds.
func (p *StatsPoller) forgetStale(now time.Time) {
	cutoff := now.Add(-max(sampleRounds*p.cfg.Interval, minSampleTTL))
	p.counters.forgetBefore(cutoff)
	p.growth.forgetBefore(cutoff)
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
		return err
	}
	var summary statsSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		return fmt.Errorf("decode kubelet summary: %w", err)
	}
	observations := p.observations(node, summary, now)
	observations = append(observations, p.kubeletMetrics(reqCtx, node, now)...)
	p.cfg.Submit(ctx, observations...)
	return nil
}

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

func (p *StatsPoller) observations(
	node inventory.EntityID, s statsSummary, now time.Time,
) []inventory.Observation {
	attrs := map[string]inventory.Value{}
	setPct(attrs, AttrFSUsedPct, s.Node.FS.UsedBytes, s.Node.FS.CapacityBytes)
	if s.Node.FS.Inodes > 0 {
		attrs[AttrInodesUsedPct] = inventory.Number(
			100 * float64(s.Node.FS.Inodes-s.Node.FS.InodesFree) /
				float64(s.Node.FS.Inodes))
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
	observations = append(observations, podUsageObservations(s, now)...)
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
)

// EnrichmentSources are the observation sources of the stats poller and the
// automatic Service prober. They only add data to entities the informers
// observe, so the model must not let them create an entity or resurrect a
// deleted one (see inventory.Options.EnrichmentSources).
func EnrichmentSources() []string {
	return []string{
		StatsSource, throttleSource, runtimeSource, serviceProbeSource,
	}
}

// podUsageObservations records container CPU and memory use and pod ephemeral
// storage use from the summary.
func podUsageObservations(
	s statsSummary, now time.Time,
) []inventory.Observation {
	var observations []inventory.Observation
	for _, pod := range s.Pods {
		ns, name := pod.PodRef.Namespace, pod.PodRef.Name
		if name == "" {
			continue
		}
		for _, c := range pod.Containers {
			if o, ok := containerUsage(
				ns, name, c.Name, c.CPU.UsageNanoCores,
				c.Memory.WorkingSetBytes, now); ok {
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

// containerUsage records one container's CPU and memory, if reported.
func containerUsage(
	ns, pod, container string, cpu, memory *uint64, now time.Time,
) (inventory.Observation, bool) {
	attrs := map[string]inventory.Value{}
	if cpu != nil {
		attrs[AttrCPUUsageMilli] = inventory.Number(
			float64(*cpu) / 1e6)
	}
	if memory != nil {
		attrs[AttrMemoryWorking] = inventory.Number(
			float64(*memory))
	}
	if len(attrs) == 0 {
		return inventory.Observation{}, false
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: StatsSource, At: now,
		Entity: ContainerID(ns, pod, container), Attributes: attrs,
	}, true
}
