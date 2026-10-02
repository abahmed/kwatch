package heartbeat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
)

// readyCheckInterval is how often a due ping rechecks readiness, so the
// first ping follows soon after monitoring becomes ready.
const readyCheckInterval = 5 * time.Second

// HeartbeatMonitor pings an external dead man's switch while kwatch is
// monitoring. A ping means "kwatch is watching the cluster", so no ping
// is sent while monitoring is not ready.
type HeartbeatMonitor struct {
	config *config.HeartbeatMonitor
	client *http.Client
	// ready reports whether monitoring is ready; nil means always.
	ready      func() bool
	readyCheck time.Duration
}

// NewHeartbeatMonitor builds a monitor with its shared HTTP client. ready
// gates every ping on monitoring readiness; nil pings unconditionally.
func NewHeartbeatMonitor(
	cfg *config.HeartbeatMonitor, client *http.Client, ready func() bool,
) *HeartbeatMonitor {
	return &HeartbeatMonitor{
		config: cfg, client: client, ready: ready,
		readyCheck: readyCheckInterval,
	}
}

// NewHeartbeatMonitorWithRuntime builds the monitor from the immutable
// runtime snapshot used by application composition.
func NewHeartbeatMonitorWithRuntime(
	runtime config.RuntimeConfig, client *http.Client, ready func() bool,
) *HeartbeatMonitor {
	cfg := runtime.Lifecycle().Heartbeat()
	return NewHeartbeatMonitor(&cfg, client, ready)
}

// StatusFunc receives the outcome of every ping: nil after a ping the
// endpoint accepted, an error otherwise. It must not block.
type StatusFunc func(error)

// Start pings until ctx ends. report, when not nil, receives the outcome
// of every ping so the application can publish heartbeat health.
func (m *HeartbeatMonitor) Start(ctx context.Context, report StatusFunc) error {
	if m.config == nil || !m.config.Enabled {
		return nil
	}
	if m.config.URL == "" {
		klog.InfoS("heartbeat monitor disabled: no URL configured")
		return nil
	}

	interval := time.Duration(m.config.Interval) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	check := time.NewTicker(m.readyCheck)
	defer check.Stop()

	klog.InfoS("heartbeat monitor started", "interval", interval)
	// A ping is due at once so a restart does not leave a gap of one
	// interval that the external monitor would report as downtime. A due
	// ping waits until monitoring is ready.
	due := true
	for {
		if due && m.monitoringReady() {
			m.reportPing(m.ping(ctx), report)
			due = false
		}
		select {
		case <-ctx.Done():
			klog.InfoS("heartbeat monitor stopped")
			return nil
		case <-ticker.C:
			due = true
		case <-check.C:
		}
	}
}

func (m *HeartbeatMonitor) monitoringReady() bool {
	return m.ready == nil || m.ready()
}

// pingTimeout bounds one ping so a hung endpoint cannot delay the next one.
const pingTimeout = 10 * time.Second

// reportPing counts a failed ping and hands the outcome to report.
func (m *HeartbeatMonitor) reportPing(err error, report StatusFunc) {
	if err != nil {
		metrics.DefaultRegistry().HeartbeatFailures.Add(1)
	}
	if report != nil {
		report(err)
	}
}

// Ping failures, as bounded errors. Details stay in the logs.
var (
	errNoClient     = errors.New("heartbeat: HTTP client is not configured")
	errPingFailed   = errors.New("heartbeat: ping failed")
	errPingRejected = errors.New("heartbeat: ping rejected")
)

func (m *HeartbeatMonitor) ping(ctx context.Context) error {
	if m.client == nil {
		klog.ErrorS(nil, "heartbeat ping skipped: HTTP client is not configured")
		return errNoClient
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, m.config.URL, nil,
	)
	if err != nil {
		klog.ErrorS(transport.RedactURLError(err),
			"heartbeat ping: failed to create request")
		return errPingFailed
	}
	resp, err := m.client.Do(req)
	if err != nil {
		// The URL often embeds a token; never log it.
		klog.ErrorS(transport.RedactURLError(err), "heartbeat ping failed")
		return errPingFailed
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	if err := resp.Body.Close(); err != nil {
		klog.ErrorS(err, "heartbeat ping: failed to close response body")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		klog.InfoS(
			"heartbeat ping returned non-2xx",
			"status",
			resp.StatusCode,
		)
		return errPingRejected
	}
	return nil
}
