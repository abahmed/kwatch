package pvc

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type PvcMonitor struct {
	client       kubernetes.Interface
	config       *config.PvcMonitor
	alertManager *alert.AlertManager
	correlator   *correlation.Engine
	notifiedPvc  map[string]bool
	mu           sync.RWMutex
	firstScan    bool
}

func NewPvcMonitor(
	client kubernetes.Interface,
	config *config.PvcMonitor,
	alertManager *alert.AlertManager,
	correlator *correlation.Engine) *PvcMonitor {
	return &PvcMonitor{
		client:       client,
		config:       config,
		alertManager: alertManager,
		correlator:   correlator,
		notifiedPvc:  make(map[string]bool),
		firstScan:    true,
	}
}

func (p *PvcMonitor) Start(ctx context.Context) {
	if !p.config.Enabled {
		return
	}

	p.checkUsage(ctx)

	ticker := time.NewTicker(time.Duration(p.config.Interval) * time.Minute)
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
		case <-cleanupTicker.C:
			p.cleanup()
		}
	}
}

func (p *PvcMonitor) reportSignal(s *event.Signal) {
	ev := event.Event{
		Resource:      s.Resource,
		PodName:       s.PodName,
		Namespace:     s.Namespace,
		Reason:        s.Reason,
		Hint:          s.Hint,
		Severity:      s.Severity,
	}
	inc, action := p.correlator.Process(ev, s.Owner, nil)
	if action != model.ActionSkip {
		p.alertManager.NotifyIncident(inc, action)
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
