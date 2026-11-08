package kube

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

// Close-call checks. When two causes score within RivalMargin of each
// other, one read-only check can tell them apart: does the endpoint's
// name resolve, and does it accept a connection. The check is the
// ones kwatch already runs on an endpoint, once, on demand: one DNS
// lookup and one TCP connect, each bounded by closeCallTimeout, to an
// address that is not private (privateAddress). It changes nothing and
// sends no payload.
const (
	closeCallTimeout = 5 * time.Second
	// closeCallRemember is how long a result stands, so that incidents
	// investigated again do not dial the endpoint again.
	closeCallRemember = 2 * time.Minute
	// maxCloseCallMemory bounds the results kept.
	maxCloseCallMemory = 64
)

// CloseCallResult is what one check of an endpoint found.
type CloseCallResult struct {
	// Endpoint is "host:port".
	Endpoint string
	Host     string
	// Resolved is true when the name has an address.
	Resolved bool
	// Connected is true when a TCP connection opened.
	Connected bool
	// TimedOut is true when the lookup or the connect got no answer in
	// closeCallTimeout: the check is then inconclusive.
	TimedOut bool
	// Blocked is true when the name resolved to an address kwatch
	// refuses to dial.
	Blocked bool
	// Refused is true when the host answered with a refusal.
	Refused bool
	// Error is the failure, already without credentials.
	Error string
}

// CloseCall checks endpoints for the investigation of a close call.
type CloseCall struct {
	resolver Resolver
	dial     func(context.Context, string, string) (net.Conn, error)
	now      func() time.Time
	mu       sync.Mutex
	last     map[string]closeCallMemory
}

type closeCallMemory struct {
	at     time.Time
	result CloseCallResult
}

// NewCloseCall builds the check. A nil dial refuses private addresses.
func NewCloseCall(
	resolver Resolver,
	dial func(context.Context, string, string) (net.Conn, error),
	now func() time.Time,
) *CloseCall {
	if dial == nil {
		dial = externalDialer()
	}
	return &CloseCall{resolver: resolver, dial: dial, now: now,
		last: map[string]closeCallMemory{}}
}

// Check looks up the endpoint's host and connects to it, once. A result
// less than closeCallRemember old is returned again instead.
func (c *CloseCall) Check(
	ctx context.Context, endpoint string,
) CloseCallResult {
	if c == nil || c.resolver == nil {
		return CloseCallResult{Endpoint: endpoint}
	}
	if memory, ok := c.remembered(endpoint); ok {
		return memory
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return CloseCallResult{Endpoint: endpoint, Error: "not a host:port"}
	}
	result := CloseCallResult{Endpoint: endpoint, Host: host}
	lookupCtx, cancel := context.WithTimeout(ctx, closeCallTimeout)
	defer cancel()
	addrs, err := c.resolver.LookupHost(lookupCtx, host)
	switch {
	case err != nil:
		result.TimedOut = timedOut(err)
		result.Error = lookupFailure(err)
	case len(addrs) == 0:
		result.Error = "no address"
	default:
		result.Resolved = true
		c.connect(ctx, &result)
	}
	c.remember(endpoint, result)
	return result
}

// connect opens one TCP connection and records how it went.
func (c *CloseCall) connect(ctx context.Context, result *CloseCallResult) {
	dialCtx, cancel := context.WithTimeout(ctx, closeCallTimeout)
	defer cancel()
	err := closeDialed(c.dial(dialCtx, "tcp", result.Endpoint))
	switch {
	case err == nil:
		result.Connected = true
	case errors.Is(err, errBlockedAddress):
		result.Blocked = true
	case timedOut(err):
		result.TimedOut = true
	default:
		result.Refused = strings.Contains(
			strings.ToLower(err.Error()), "refused")
		result.Error = evidenceText(err.Error())
	}
}

func timedOut(err error) bool {
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout())
}

// lookupFailure is the short reason a name did not resolve.
func lookupFailure(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return "no such host"
	}
	return "lookup failed"
}

func (c *CloseCall) remembered(endpoint string) (CloseCallResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.last[endpoint]
	if !ok || c.now().Sub(m.at) >= closeCallRemember {
		return CloseCallResult{}, false
	}
	return m.result, true
}

func (c *CloseCall) remember(endpoint string, result CloseCallResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.last) >= maxCloseCallMemory {
		c.last = map[string]closeCallMemory{}
	}
	c.last[endpoint] = closeCallMemory{at: c.now(), result: result}
}
