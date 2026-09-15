package pvc

import (
	"context"
	"time"

	"k8s.io/klog/v2"
)

func (p *PvcMonitor) Start(ctx context.Context) {
	if !p.config.Enabled {
		return
	}
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()
	p.restore(ctx)
	p.checkUsage(ctx)
	p.persist(ctx)

	interval := p.config.Interval.Duration()
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	cleanupTicker := time.NewTicker(time.Hour)
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

func (p *PvcMonitor) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	count := len(p.notifiedPvc)
	if count > 1000 {
		klog.V(4).InfoS(
			"pvc monitor: clearing stale entries from notifiedPvc cache",
			"count", count,
		)
		p.notifiedPvc = make(map[string]bool)
	}
}
