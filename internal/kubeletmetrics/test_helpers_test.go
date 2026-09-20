package kubeletmetrics

import (
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

func newTestMonitor(
	client kubernetes.Interface,
	cfg config.KubeletTelemetryMonitor,
	sink monitor.ObservationSink,
) *Monitor {
	return NewWithClock(client, cfg, sink, clock.RealClock{})
}

func setTestOwnerResolver(
	monitor *Monitor,
	resolver observe.OwnerResolver,
) {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	monitor.owners = resolver
}
