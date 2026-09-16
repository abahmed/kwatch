package probe

import (
	"net/http"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

func newTestMonitor(
	cfg config.ActiveProbeMonitor,
	sink monitor.ObservationSink,
	client *http.Client,
	resolvers ...HostResolver,
) *Monitor {
	var resolver HostResolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	return NewWithDependencies(
		cfg, sink, client, resolver, clock.RealClock{},
	)
}

func setTestResolver(monitor *Monitor, resolver HostResolver) {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	monitor.resolver = resolver
}
