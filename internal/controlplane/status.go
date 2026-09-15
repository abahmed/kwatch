package controlplane

import (
	"context"
	"errors"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type EndpointStatus struct {
	Name        string        `json:"name"`
	Available   bool          `json:"available"`
	Latency     time.Duration `json:"latency"`
	LastError   string        `json:"lastError,omitempty"`
	LastChecked time.Time     `json:"lastChecked"`
	Supported   bool          `json:"supported"`
}

type Status struct {
	State       string                    `json:"state"`
	LastCheck   time.Time                 `json:"lastCheck"`
	APIServer   EndpointStatus            `json:"apiServer"`
	CoreDNS     EndpointStatus            `json:"coreDNS"`
	Components  map[string]EndpointStatus `json:"components"`
	ProbeErrors int64                     `json:"probeErrors"`
}

func controlPlaneState(status Status) string {
	if !status.APIServer.Supported && !status.CoreDNS.Supported {
		return "unavailable"
	}
	if !status.APIServer.Available ||
		(status.CoreDNS.Supported && !status.CoreDNS.Available) {
		return "partial"
	}
	for _, component := range status.Components {
		if component.Supported && !component.Available {
			return "partial"
		}
	}
	if status.LastCheck.IsZero() {
		return "unavailable"
	}
	return "healthy"
}

// safeProbeError keeps arbitrary transport details out of diagnostics. The
// detailed error remains available in structured logs and incident evidence.
func safeProbeError(err error) string {
	if err == nil {
		return "probe_failed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if apierrors.IsForbidden(err) {
		return "permission_denied"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "dns") {
		return "dns_failed"
	}
	if strings.Contains(message, "connection") {
		return "connection_failed"
	}
	return "probe_failed"
}
