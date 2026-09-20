package probe

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
)

func (m *Monitor) http(
	ctx context.Context,
	target config.HTTPProbeTarget,
) (bool, string, string) {
	started := m.nowTime()
	probeCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		probeCtx, http.MethodGet, target.URL, nil,
	)
	if err != nil {
		return false, err.Error(), constant.ReasonActiveProbeFailure
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return false, err.Error(), constant.ReasonActiveProbeFailure
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	latency := m.nowTime().Sub(started)
	expected := target.ExpectedStatus
	if expected == 0 {
		if resp.StatusCode >= http.StatusOK &&
			resp.StatusCode < http.StatusBadRequest {
			return probeLatencyResult(resp.StatusCode, latency, target)
		}
		return false, fmt.Sprintf(
			"HTTP status %d (expected 2xx or 3xx)", resp.StatusCode,
		), constant.ReasonActiveProbeFailure
	}
	if resp.StatusCode != expected {
		return false, fmt.Sprintf(
			"HTTP status %d (expected %d)", resp.StatusCode, expected,
		), constant.ReasonActiveProbeFailure
	}
	return probeLatencyResult(resp.StatusCode, latency, target)
}

func probeLatencyResult(
	status int,
	latency time.Duration,
	target config.HTTPProbeTarget,
) (bool, string, string) {
	detail := fmt.Sprintf(
		"HTTP status %d in %s", status, latency.Round(time.Millisecond),
	)
	if target.LatencyCriticalMs > 0 &&
		latency >= time.Duration(target.LatencyCriticalMs)*time.Millisecond {
		return false, detail + fmt.Sprintf(
			" (critical latency threshold %dms exceeded)",
			target.LatencyCriticalMs,
		), constant.ReasonActiveProbeLatency
	}
	if target.LatencyWarningMs > 0 &&
		latency >= time.Duration(target.LatencyWarningMs)*time.Millisecond {
		return false, detail + fmt.Sprintf(
			" (latency threshold %dms exceeded)",
			target.LatencyWarningMs,
		), constant.ReasonActiveProbeLatency
	}
	return true, detail, constant.ReasonActiveProbeFailure
}

func (m *Monitor) tcp(
	ctx context.Context,
	target config.TCPProbeTarget,
) (bool, string) {
	dialer := net.Dialer{Timeout: m.timeout}
	probeCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	conn, err := dialer.DialContext(probeCtx, "tcp", target.Address)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, "TCP connection established"
}

func (m *Monitor) dns(
	ctx context.Context,
	target config.DNSProbeTarget,
) (bool, string) {
	probeCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	m.mu.Lock()
	resolver := m.resolver
	m.mu.Unlock()
	if resolver == nil {
		return false, "DNS resolver is not configured"
	}
	addresses, err := resolver.LookupHost(probeCtx, target.Host)
	if err != nil {
		return false, err.Error()
	}
	if len(addresses) == 0 {
		return false, "DNS returned no addresses"
	}
	return true, fmt.Sprintf(
		"DNS resolved to %d address(es)", len(addresses),
	)
}
