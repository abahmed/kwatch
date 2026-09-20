package resource

import (
	"context"
	"time"

	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/model"
)

func NewMonitor(
	cfg Config,
	nodeLister corev1lister.NodeLister,
	podLister corev1lister.PodLister,
) *Monitor {
	return &Monitor{
		cfg: cfg, nodeLister: nodeLister, podLister: podLister,
		client: cfg.Client, interval: cfg.Interval,
	}
}

// Run starts the periodic check loop. The callback receives each signal.
func (m *Monitor) Run(
	ctx context.Context, callback func(obs *model.Observation),
) {
	if m.interval <= 0 {
		m.interval = defaultCheckInterval
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			signals := m.Check()
			signals = append(signals, m.checkFilesystem(ctx)...)
			for _, sig := range signals {
				callback(sig)
			}
		}
	}
}
