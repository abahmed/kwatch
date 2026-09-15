package probe

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
)

func (m *Monitor) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		return
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()
	interval := time.Duration(m.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	m.check(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) check(ctx context.Context) {
	for _, target := range m.cfg.HTTP {
		m.linkTarget("http/"+target.Name, target.URL)
		ok, detail, reason := m.http(ctx, target)
		m.record("http/"+target.Name, "http/"+target.Name, reason, ok, detail)
	}
	for _, target := range m.cfg.TCP {
		m.linkTarget("tcp/"+target.Name, target.Address)
		ok, detail := m.tcp(ctx, target)
		m.record(
			"tcp/"+target.Name, "tcp/"+target.Name,
			constant.ReasonActiveProbeFailure, ok, detail,
		)
	}
	for _, target := range m.cfg.DNS {
		m.linkTarget("dns/"+target.Name, target.Host)
		ok, detail := m.dns(ctx, target)
		m.record(
			"dns/"+target.Name, "dns/"+target.Name,
			constant.ReasonActiveProbeFailure, ok, detail,
		)
	}
	if m.cfg.AutoServices && m.kclient != nil {
		m.checkServices(ctx)
	}
}
