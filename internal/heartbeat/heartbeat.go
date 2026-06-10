package heartbeat

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type HeartbeatMonitor struct {
	correlator   *correlation.Engine
	alertManager *alert.AlertManager
	config       *config.HeartbeatMonitor
}

func NewHeartbeatMonitor(
	correlator *correlation.Engine,
	alertManager *alert.AlertManager,
	cfg *config.HeartbeatMonitor,
) *HeartbeatMonitor {
	return &HeartbeatMonitor{
		correlator:   correlator,
		alertManager: alertManager,
		config:       cfg,
	}
}

func (m *HeartbeatMonitor) Start(ctx context.Context) {
	if !m.config.Enabled {
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
			m.beat()
		}
	}
}

func (m *HeartbeatMonitor) beat() {
	ev := event.Event{
		Resource: "heartbeat",
		PodName:  "deadmansswitch",
		Reason:   "Heartbeat",
	}
	inc, action := m.correlator.Process(ev, "deadmansswitch", nil)
	if action != model.ActionSkip {
		m.alertManager.NotifyIncident(inc, action)
	}
}
