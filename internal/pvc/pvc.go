package pvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
	"github.com/abahmed/kwatch/internal/state"
)

const (
	maxConcurrentSamples = 10
	maxConcurrentNodeOps = 5
)

type PvcMonitor struct {
	client              kubernetes.Interface
	config              *config.PvcMonitor
	correlator          *correlation.Engine
	state               *state.StateManager // persistence; nil only in unit tests
	notifiedPvc         map[string]bool
	lastUsage           map[string]state.PvcSample // last observed sample per PV name (survives unmount)
	pvByPVC             map[string]string          // cached PVC→PV map (shared by sweep + per-node sample)
	pvByPVCAt           time.Time                  // when pvByPVC was last refreshed
	now                 func() time.Time
	mu                  sync.RWMutex
	firstScan           bool
	sem                 chan struct{}                                                                              // bounds concurrent getNodeUsage calls
	getNodeUsageFn      func(ctx context.Context, nodeName string, pvByPVC map[string]string) ([]*PvcUsage, error) // test override
	allowedNamespaces   map[string]struct{}
	forbiddenNamespaces map[string]struct{}
	namespaceFilter     func(string) bool
	watchAll            bool
}

// SetNamespaceScope keeps the periodic API-based storage checks aligned with
// the informer scope used by the controller. Empty scope means all namespaces.
func (p *PvcMonitor) SetNamespaceScope(
	allowed, forbidden []string,
	all ...bool,
) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowedNamespaces = make(map[string]struct{}, len(allowed))
	for _, namespace := range allowed {
		p.allowedNamespaces[namespace] = struct{}{}
	}
	p.forbiddenNamespaces = make(map[string]struct{}, len(forbidden))
	for _, namespace := range forbidden {
		p.forbiddenNamespaces[namespace] = struct{}{}
	}
	p.watchAll = len(allowed) == 0
	if len(all) > 0 {
		p.watchAll = all[0]
	}
	p.pvByPVC = nil
	p.pvByPVCAt = time.Time{}
}

func (p *PvcMonitor) listPVCs(
	ctx context.Context,
) ([]corev1.PersistentVolumeClaim, error) {
	p.mu.RLock()
	namespaces := make([]string, 0, len(p.allowedNamespaces))
	for namespace := range p.allowedNamespaces {
		namespaces = append(namespaces, namespace)
	}
	watchAll := p.watchAll
	p.mu.RUnlock()
	if watchAll {
		namespaces = []string{""}
	}
	var result []corev1.PersistentVolumeClaim
	for _, namespace := range namespaces {
		continueToken := ""
		for {
			list, err := p.client.CoreV1().PersistentVolumeClaims(namespace).List(
				ctx, metav1.ListOptions{Limit: 500, Continue: continueToken},
			)
			if err != nil {
				return nil, err
			}
			result = append(result, list.Items...)
			continueToken = list.Continue
			if continueToken == "" {
				break
			}
		}
	}
	return result, nil
}

// SetNamespaceFilter lets the controller provide its resolved namespace
// selector, including dynamic NamespaceSelector configuration.
func (p *PvcMonitor) SetNamespaceFilter(filter func(string) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.namespaceFilter = filter
	p.pvByPVC = nil
	p.pvByPVCAt = time.Time{}
}

func (p *PvcMonitor) namespaceAllowed(namespace string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.namespaceAllowedLocked(namespace)
}

func (p *PvcMonitor) namespaceAllowedLocked(namespace string) bool {
	if p.namespaceFilter != nil {
		return p.namespaceFilter(namespace)
	}
	if _, forbidden := p.forbiddenNamespaces[namespace]; forbidden {
		return false
	}
	if len(p.allowedNamespaces) > 0 {
		_, allowed := p.allowedNamespaces[namespace]
		return allowed
	}
	return true
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
	if pvcs, err := p.listPVCs(ctx); err == nil {
		for i := range pvcs {
			c := &pvcs[i]
			if !p.namespaceAllowed(c.Namespace) {
				continue
			}
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
		watchAll:    true,
		sem:         make(chan struct{}, maxConcurrentSamples),
	}
}

// SetClock injects the clock used for PVC cache TTLs and lifecycle decisions.
func (p *PvcMonitor) SetClock(now func() time.Time) {
	if now != nil {
		p.mu.Lock()
		p.now = now
		p.mu.Unlock()
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
		// Samples used to be keyed by PV name; re-key from the sample's own
		// claim identity so a restart across that change neither loses the
		// held state nor resurrects an entry the live path can never clear.
		p.lastUsage = rekeyByClaim(p.lastUsage)
		var restore []*model.Observation
		for key, s := range p.lastUsage {
			if !p.namespaceAllowedLocked(s.Namespace) {
				delete(p.lastUsage, key)
				delete(p.notifiedPvc, key)
				continue
			}
			if s.Pct >= p.config.Threshold {
				p.notifiedPvc[key] = true
				sev := model.SeverityNormal
				if s.Pct >= p.config.CriticalThreshold {
					sev = model.SeverityHigh
				}
				restore = append(restore, observe.VolumeUsage(
					s.Namespace, s.Name, s.PodName,
					constant.ReasonVolumeUsageHigh,
				).WithSeverity(sev).WithFacts(volumeFacts(s.PVName)).
					WithHint(fmt.Sprintf("VolumeUsage(%.0f%%)", s.Pct)))
			}
		}
		p.mu.Unlock()
		for _, obs := range restore {
			p.report(obs)
		}
	}

	p.checkUsage(ctx)
	p.persist(ctx) // B3: persist the initial sweep too (previously only the ticker loop persisted)

	interval := p.config.Interval.Duration()
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

// rekeyByClaim rebuilds a usage map under "namespace/claim" keys, taking the
// identity from each sample rather than from its existing key. Samples with no
// recorded claim are dropped: they cannot be matched to a live claim, so
// keeping them would hold an incident that nothing can ever resolve.
func rekeyByClaim(
	usage map[string]state.PvcSample,
) map[string]state.PvcSample {
	result := make(map[string]state.PvcSample, len(usage))
	for _, sample := range usage {
		if sample.Namespace == "" || sample.Name == "" {
			continue
		}
		key := sample.Namespace + "/" + sample.Name
		if existing, ok := result[key]; ok && existing.Seen.After(sample.Seen) {
			continue
		}
		result[key] = sample
	}
	return result
}

// volumeFacts records the bound PersistentVolume as evidence on a claim-keyed
// incident, so the PV name stays visible in the notification without being
// part of the incident's identity.
func volumeFacts(pvName string) model.Facts {
	return model.Facts{Volume: pvName}
}

// report forwards a storage observation to the engine.
//
// This used to be its own hand-written conversion into the pipeline's event,
// and it dropped labels and pod identity: every silence rule matching on
// labels or pod-name patterns was silently inert for storage incidents, and
// the engine could not tell a replacement Pod from the original.
func (p *PvcMonitor) report(obs *model.Observation) {
	p.correlator.Process(obs)
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
