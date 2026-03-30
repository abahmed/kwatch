package pvc

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type PvcMonitor struct {
	client       kubernetes.Interface
	config       *config.PvcMonitor
	alertManager *alert.AlertManager
	notifiedPvc  map[string]bool
	mu           sync.RWMutex
}

func NewPvcMonitor(
	client kubernetes.Interface,
	config *config.PvcMonitor,
	alertManager *alert.AlertManager) *PvcMonitor {
	return &PvcMonitor{
		client:       client,
		config:       config,
		alertManager: alertManager,
		notifiedPvc:  make(map[string]bool),
	}
}

func (p *PvcMonitor) Start() {
	if !p.config.Enabled {
		return
	}

	p.checkUsage()

	ticker := time.NewTicker(time.Duration(p.config.Interval) * time.Minute)
	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ticker.C:
			p.checkUsage()
		case <-cleanupTicker.C:
			p.cleanup()
		}
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
