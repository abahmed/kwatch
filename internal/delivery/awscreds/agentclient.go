package awscreds

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// The EKS Pod Identity agent's link-local addresses. These are the only
// metadata-range addresses kwatch connects to; every other one stays
// refused (see transport.GuardMetadata).
var agentAddrs = []net.IP{
	net.ParseIP("169.254.170.23"),
	net.ParseIP("fd00:ec2::23"),
}

// errAgentAddress is returned for a connection to an address that is
// neither a Pod Identity agent address nor loopback.
var errAgentAddress = errors.New(
	"refused: not the EKS Pod Identity agent address or loopback")

// agentIP reports whether ip is an agent address or loopback.
func agentIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, allowed := range agentAddrs {
		if ip.Equal(allowed) {
			return true
		}
	}
	return false
}

// CheckAgentURL rejects a Pod Identity URL whose host is not an agent
// address, a loopback address or localhost.
func CheckAgentURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("AWS_CONTAINER_CREDENTIALS_FULL_URI is not " +
			"an http or https URL")
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") ||
		agentIP(net.ParseIP(host)) {
		return nil
	}
	return fmt.Errorf("AWS_CONTAINER_CREDENTIALS_FULL_URI host %q is "+
		"not allowed: only the EKS Pod Identity agent (169.254.170.23, "+
		"fd00:ec2::23) or a loopback address", host)
}

// agentTimeout bounds one call to the agent, which answers locally.
const agentTimeout = 5 * time.Second

// newAgentClient is the HTTP client for the Pod Identity agent. It
// connects only to agent or loopback addresses, checked on the address
// being dialed (after name resolution), never uses a proxy and never
// follows a redirect, so the agent token cannot travel anywhere else.
func newAgentClient() *http.Client {
	dialer := &net.Dialer{Timeout: agentTimeout,
		ControlContext: agentControl}
	return &http.Client{
		Timeout: agentTimeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			ResponseHeaderTimeout: agentTimeout,
			MaxIdleConns:          1,
			IdleConnTimeout:       time.Minute,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("the Pod Identity agent redirected; " +
				"kwatch does not follow redirects")
		},
	}
}

func agentControl(
	_ context.Context, _, address string, _ syscall.RawConn,
) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if !agentIP(net.ParseIP(host)) {
		return errAgentAddress
	}
	return nil
}
