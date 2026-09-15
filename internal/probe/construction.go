package probe

import (
	"context"
	"net/http"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// HostResolver is the DNS dependency used by DNS probes.
type HostResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

// NewWithDependencies constructs a probe monitor with all external
// dependencies supplied by application composition.
func NewWithDependencies(
	cfg config.ActiveProbeMonitor,
	incidentSink monitor.ObservationSink,
	sharedClient *http.Client,
	resolver HostResolver,
	timeSource clock.Clock,
) *Monitor {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if timeSource == nil {
		timeSource = clock.RealClock{}
	}
	return &Monitor{
		cfg: cfg, incidentSink: incidentSink,
		watchAll: true,
		client:   sharedClient,
		timeout:  timeout,
		failures: make(map[string]int), successes: make(map[string]int),
		failing:     make(map[string]bool),
		autoTargets: make(map[string]autoProbeTarget),
		now:         timeSource.Now,
		resolver:    resolver,
	}
}

func (m *Monitor) nowTime() time.Time {
	return m.now()
}
