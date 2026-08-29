package pvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/state"
)

const (
	maxConcurrentSamples = 10
	maxConcurrentNodeOps = 5
)

type PvcMonitor struct {
	client         kubernetes.Interface
	config         *config.PvcMonitor
	correlator     *correlation.Engine
	state          *state.StateManager // persistence; nil only in unit tests
	notifiedPvc    map[string]bool
	lastUsage      map[string]state.PvcSample // last observed sample per PV name (survives unmount)
	pvByPVC        map[string]string          // cached PVC→PV map (shared by sweep + per-node sample)
	pvByPVCAt      time.Time                  // when pvByPVC was last refreshed
	now            func() time.Time
	mu             sync.RWMutex
	firstScan      bool
	sem            chan struct{}                                                                              // bounds concurrent getNodeUsage calls
	getNodeUsageFn func(ctx context.Context, nodeName string, pvByPVC map[string]string) ([]*PvcUsage, error) // test override
}

const pvByPVCTTL = 60 * time.Second

// pvcMap returns the PVC→PV name map, refreshing from the API server at most
// once per pvByPVCTTL. Shared by checkUsage (sweep) and per-node sample (event-driven).
func (p *PvcMonitor) pvcMap(ctx context.Context) map[string]string {
	p.mu.RLock()
	if p.pvByPVC != nil && p.now().Sub(p.pvByPVCAt) < pvByPVCTTL {
		m := p.pvByPVC
		p.mu.RUnlock()
		return m
	}
	p.mu.RUnlock()

	m := make(map[string]string)
	if p.client == nil {
		return m
	}
	if pvcs, err := p.client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{}); err == nil {
		for i := range pvcs.Items {
			c := &pvcs.Items[i]
			m[c.Namespace+"/"+c.Name] = c.Spec.VolumeName
		}
		p.mu.Lock()
		p.pvByPVC, p.pvByPVCAt = m, p.now()
		p.mu.Unlock()
	} else {
		klog.ErrorS(err, "pvc monitor: list PVCs")
		p.mu.RLock()
		m = p.pvByPVC // fall back to last good map
		p.mu.RUnlock()
	}
	return m
}

func NewPvcMonitor(
	client kubernetes.Interface,
	config *config.PvcMonitor,
	correlator *correlation.Engine,
	stateMgr *state.StateManager,
) *PvcMonitor {
	return &PvcMonitor{
		client:      client,
		config:      config,
		correlator:  correlator,
		state:       stateMgr,
		notifiedPvc: make(map[string]bool),
		lastUsage:   make(map[string]state.PvcSample),
		now:         time.Now,
		firstScan:   true,
		sem:         make(chan struct{}, maxConcurrentSamples),
	}
}

func (p *PvcMonitor) Start(ctx context.Context) {
	if p.config == nil || !p.config.Enabled {
		return
	}

	// Seed the in-memory cache from persisted state so a restart keeps
	// firing on high-but-unmounted PVCs without waiting for a re-mount.
	// notifiedPvc is re-derived from lastUsage rather than persisted separately
	// — avoids the phantom held-state problem (B2) and saves a ConfigMap key.
	if p.state != nil {
		seed := p.state.GetPvcUsage(ctx)
		p.mu.Lock()
		if seed != nil {
			p.lastUsage = seed
		}
		var restore []*event.Signal
		for pv, s := range p.lastUsage {
			if s.Pct >= p.config.Threshold {
				p.notifiedPvc[pv] = true
				sev := model.SeverityNormal
				if s.Pct >= p.config.CriticalThreshold {
					sev = model.SeverityHigh
				}
				restore = append(restore, &event.Signal{
					Resource: "pvc", PodName: s.PodName, Namespace: s.Namespace,
					Reason: constant.ReasonVolumeUsageHigh, Hint: fmt.Sprintf("VolumeUsage(%.0f%%)", s.Pct),
					Severity: sev, Owner: pv,
				})
			}
		}
		p.mu.Unlock()
		for _, sig := range restore {
			p.reportSignal(sig)
		}
	}

	p.checkUsage(ctx)
	p.persist(ctx) // B3: persist the initial sweep too (previously only the ticker loop persisted)

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
	p.correlator.Process(ev, s.Owner, nil)
}

// persist snapshots lastUsage to the kwatch-pvc ConfigMap. Called ONLY from the
// periodic sweep only, not from per-node samples — otherwise a burst of Running
// pods would write etcd on every sample (write-amplification). A crash loses at
// most the deltas since the last sweep (≤ interval), and the next sweep
// re-observes every mounted volume anyway.
// notifiedPvc is intentionally not persisted — it is re-derived from lastUsage on seed.
func (p *PvcMonitor) persist(ctx context.Context) {
	if p.state == nil {
		return
	}
	p.mu.RLock()
	usage := make(map[string]state.PvcSample, len(p.lastUsage))
	for k, v := range p.lastUsage {
		usage[k] = v
	}
	p.mu.RUnlock()
	if err := p.state.SavePvcUsage(ctx, usage); err != nil {
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
