package pvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/state"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const nodeSampleDebounce = 30 * time.Second

type PvcMonitor struct {
	client         kubernetes.Interface
	config         *config.PvcMonitor
	alertManager   *alert.AlertManager
	correlator     *correlation.Engine
	state          *state.StateManager // persistence; nil only in unit tests
	notifiedPvc    map[string]bool
	lastUsage      map[string]state.PvcSample // last observed sample per PV name (survives unmount)
	lastNodeSample map[string]time.Time       // per-node SampleNode debounce
	mu             sync.RWMutex
	firstScan      bool
	getNodeUsageFn func(ctx context.Context, nodeName string, pvByPVC map[string]string) ([]*PvcUsage, error) // test override
}

func NewPvcMonitor(
	client kubernetes.Interface,
	config *config.PvcMonitor,
	alertManager *alert.AlertManager,
	correlator *correlation.Engine,
	stateMgr *state.StateManager,
) *PvcMonitor {
	return &PvcMonitor{
		client:       client,
		config:       config,
		alertManager: alertManager,
		correlator:   correlator,
		state:        stateMgr,
		notifiedPvc:  make(map[string]bool),
		lastUsage:    make(map[string]state.PvcSample),
		firstScan:    true,
	}
}

func (p *PvcMonitor) Start(ctx context.Context) {
	if !p.config.Enabled {
		return
	}

	// Seed the in-memory cache from persisted state so a restart keeps
	// firing on high-but-unmounted PVCs without waiting for a re-mount.
	if p.state != nil {
		if seed := p.state.GetPvcUsage(ctx); seed != nil {
			p.mu.Lock()
			p.lastUsage = seed
			var restore []*event.Signal
			for pv, s := range seed {
				if s.Pct >= p.config.Threshold {
					p.notifiedPvc[pv] = true
					sev := "normal"
					if s.Pct >= p.config.CriticalThreshold {
						sev = "high"
					}
					restore = append(restore, &event.Signal{
						Resource: "pvc", PodName: s.Name, Namespace: s.Namespace,
						Reason: "VolumeUsageHigh", Hint: fmt.Sprintf("VolumeUsage(%.0f%%)", s.Pct),
						Severity: sev, Owner: pv,
					})
				}
			}
			p.mu.Unlock()
			for _, sig := range restore {
				p.reportSignal(sig)
			}
		}
	}

	p.checkUsage(ctx)

	interval := time.Duration(p.config.Interval) * time.Minute
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.InfoS("pvc monitor stopped")
			return
		case <-ticker.C:
			p.checkUsage(ctx)
			p.persist(ctx)
		case <-cleanupTicker.C:
			p.cleanup()
		}
	}
}

func (p *PvcMonitor) reportSignal(s *event.Signal) {
	ev := event.Event{
		Resource:  s.Resource,
		PodName:   s.PodName,
		Namespace: s.Namespace,
		Reason:    s.Reason,
		Hint:      s.Hint,
		Severity:  s.Severity,
	}
	inc, action := p.correlator.Process(ev, s.Owner, nil)
	if action != model.ActionSkip {
		p.alertManager.NotifyIncident(inc, action)
	}
}

// persist snapshots lastUsage to the kwatch-pvc ConfigMap.
// Called ONLY from the periodic sweep, not from SampleNode — otherwise a burst of
// Running pods would write etcd on every sample (write-amplification). A crash loses
// at most the SampleNode deltas since the last sweep (≤ interval), and the next sweep
// re-observes every mounted volume anyway.
func (p *PvcMonitor) persist(ctx context.Context) {
	if p.state == nil {
		return
	}
	p.mu.RLock()
	snapshot := make(map[string]state.PvcSample, len(p.lastUsage))
	for k, v := range p.lastUsage {
		snapshot[k] = v
	}
	p.mu.RUnlock()
	if err := p.state.SavePvcUsage(ctx, snapshot); err != nil {
		klog.ErrorS(err, "pvc monitor: persist usage failed")
	}
}

func (p *PvcMonitor) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	count := len(p.notifiedPvc)
	if count > 1000 {
		klog.V(4).InfoS("pvc monitor: clearing stale entries from notifiedPvc cache", "count", count)
		p.notifiedPvc = make(map[string]bool)
	}
}
