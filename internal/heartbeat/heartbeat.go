package heartbeat

import (
	"context"
	"net/http"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"k8s.io/klog/v2"
)

type HeartbeatMonitor struct {
	config *config.HeartbeatMonitor
	client *http.Client
}

func NewHeartbeatMonitor(cfg *config.HeartbeatMonitor) *HeartbeatMonitor {
	return &HeartbeatMonitor{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *HeartbeatMonitor) Start(ctx context.Context) {
	if !m.config.Enabled {
		return
	}
	if m.config.URL == "" {
		klog.InfoS("heartbeat monitor disabled: no URL configured")
		return
	}

	interval := time.Duration(m.config.Interval) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	klog.InfoS("heartbeat monitor started", "interval", interval, "url", m.config.URL)
	for {
		select {
		case <-ctx.Done():
			klog.InfoS("heartbeat monitor stopped")
			return
		case <-ticker.C:
			m.ping()
		}
	}
}

func (m *HeartbeatMonitor) ping() {
	resp, err := m.client.Get(m.config.URL)
	if err != nil {
		klog.ErrorS(err, "heartbeat ping failed", "url", m.config.URL)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		klog.InfoS("heartbeat ping returned non-2xx", "status", resp.StatusCode, "url", m.config.URL)
	}
}
