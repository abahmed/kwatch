package kube

import (
	"context"
	"errors"
	"net"
	"syscall"
)

// How a probe failed, recorded beside the error text so readers never
// parse it. A failure of none of these kinds records no kind.
const (
	AttrProbeFailureKind = "probe.failure.kind"
	// AttrProbeTimeoutSeconds is how long the probe waited, so "did not
	// answer within 3s" can say 3s.
	AttrProbeTimeoutSeconds = "probe.timeout.seconds"

	FailureTimeout = "timeout"
	FailureRefused = "refused"
	FailureDNS     = "dns"
	// FailureDNSLookup is a lookup that failed some other way than
	// "no such name": a server failure or refusal, not a missing record.
	FailureDNSLookup = "dns-lookup"
)

// probeFailureKind tells a timeout from a refusal and from a name that
// does not resolve. Each points to a different problem: a silent
// firewall, a service that is down, a wrong or missing DNS record.
func probeFailureKind(err error) string {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case err == nil:
		return ""
	case errors.As(err, &dnsErr):
		if dnsErr.IsTimeout {
			return FailureTimeout
		}
		if dnsErr.IsNotFound {
			return FailureDNS
		}
		return FailureDNSLookup
	case errors.Is(err, syscall.ECONNREFUSED):
		return FailureRefused
	case errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		return FailureTimeout
	}
	return ""
}
