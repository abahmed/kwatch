package heartbeat

import (
	"context"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

type HeartbeatMonitor struct {
	config *config.HeartbeatMonitor
	client *http.Client
}

// NewHeartbeatMonitor builds a monitor with an optional shared HTTP client.
// The default keeps the constructor backward compatible for callers outside
// the application composition root.
func NewHeartbeatMonitor(
	cfg *config.HeartbeatMonitor, clients ...*http.Client,
) *HeartbeatMonitor {
	client := http.DefaultClient
	if len(clients) > 0 && clients[0] != nil {
		client = clients[0]
	}
	return &HeartbeatMonitor{
		config: cfg,
		client: client,
	}
}

func (m *HeartbeatMonitor) Start(ctx context.Context) {
	if m.config == nil || !m.config.Enabled {
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

	klog.InfoS("heartbeat monitor started", "interval", interval)
	for {
		select {
		case <-ctx.Done():
			klog.InfoS("heartbeat monitor stopped")
			return
		case <-ticker.C:
			m.ping(ctx)
		}
	}
}

func (m *HeartbeatMonitor) ping(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.config.URL, nil)
	if err != nil {
		klog.ErrorS(err, "heartbeat ping: failed to create request")
		return
	}
	resp, err := m.client.Do(req)
	if err != nil {
		klog.ErrorS(err, "heartbeat ping failed")
		return
	}
	if err := resp.Body.Close(); err != nil {
		klog.ErrorS(err, "heartbeat ping: failed to close response body")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		klog.InfoS(
			"heartbeat ping returned non-2xx",
			"status",
			resp.StatusCode,
		)
	}
}
