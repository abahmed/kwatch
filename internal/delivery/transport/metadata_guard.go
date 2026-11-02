package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
)

// ErrMetadataAddress is the error of a send whose destination is a cloud
// instance-metadata address. Such an endpoint hands out the node's cloud
// credentials, so a webhook URL pointing there is refused for good.
var ErrMetadataAddress = errors.New("refused: cloud metadata address")

// metadataNames are the host names of cloud metadata services.
var metadataNames = map[string]bool{
	"metadata.google.internal": true,
	"metadata.goog":            true,
}

// metadataIPs are metadata addresses outside the link-local ranges:
// AWS over IPv6 and Alibaba Cloud.
var metadataIPs = []net.IP{
	net.ParseIP("fd00:ec2::254"),
	net.ParseIP("100.100.100.200"),
}

// nat64 is the well-known NAT64 prefix, which can carry an IPv4 metadata
// address inside IPv6.
var nat64 = &net.IPNet{IP: net.ParseIP("64:ff9b::"),
	Mask: net.CIDRMask(96, 128)}

// MetadataAddress reports an address a provider send must not reach:
// the link-local ranges (169.254.0.0/16 and fe80::/10, which hold
// 169.254.169.254) and the other cloud metadata addresses. Private,
// loopback and cluster addresses are allowed: in-cluster chat servers
// live there.
func MetadataAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if nat64.Contains(ip) {
		ip = net.IP(ip.To16()[12:16])
	}
	if ip.IsLinkLocalUnicast() {
		return true
	}
	for _, m := range metadataIPs {
		if m.Equal(ip) {
			return true
		}
	}
	return false
}

// metadataHost reports a URL host (a name or an address) that names a
// metadata service.
func metadataHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if metadataNames[host] {
		return true
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	return MetadataAddress(net.ParseIP(host))
}

// Resolver looks up the addresses of a host name.
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

type dialFunc = func(context.Context, string, string) (net.Conn, error)

// GuardMetadata returns a copy of c whose requests never reach a cloud
// metadata address: a URL naming one is refused before any I/O, and when
// the transport is an *http.Transport its connections dial only
// addresses checked after the name was resolved, so a name that resolves
// (or later rebinds) to a metadata address is refused too. A nil c stays
// nil.
func GuardMetadata(c *http.Client) *http.Client {
	return guardMetadata(c, &net.Resolver{})
}

func guardMetadata(c *http.Client, resolver Resolver) *http.Client {
	if c == nil {
		return nil
	}
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if t, ok := base.(*http.Transport); ok {
		clone := t.Clone()
		dial := clone.DialContext
		if dial == nil {
			dial = (&net.Dialer{}).DialContext
		}
		clone.DialContext = metadataDial(resolver, dial)
		base = clone
	}
	copied := *c
	copied.Transport = metadataTransport{base: base}
	return &copied
}

// metadataTransport refuses a request whose URL names a metadata host.
type metadataTransport struct {
	base http.RoundTripper
}

func (t metadataTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if metadataHost(r.URL.Hostname()) {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, Permanent(ErrMetadataAddress)
	}
	return t.base.RoundTrip(r)
}

// metadataDial resolves a name here, refuses it when any answer is a
// metadata address, and dials the addresses it checked in turn, so the
// name cannot answer differently at connect time.
func metadataDial(resolver Resolver, dial dialFunc) dialFunc {
	return func(
		ctx context.Context, network, address string,
	) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		answers, err := resolveChecked(ctx, resolver, host)
		if err != nil {
			return nil, err
		}
		var last error
		for _, answer := range answers {
			conn, err := dial(ctx, network, net.JoinHostPort(answer, port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
}

// resolveChecked returns the addresses of host, refusing a metadata
// name or any metadata answer.
func resolveChecked(
	ctx context.Context, resolver Resolver, host string,
) ([]string, error) {
	if metadataHost(host) {
		return nil, Permanent(ErrMetadataAddress)
	}
	if net.ParseIP(host) != nil {
		return []string{host}, nil
	}
	answers, err := resolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(answers) == 0 {
		return nil, &net.DNSError{Err: "no addresses", Name: host,
			IsNotFound: true}
	}
	for _, answer := range answers {
		if MetadataAddress(net.ParseIP(answer)) {
			return nil, Permanent(ErrMetadataAddress)
		}
	}
	return answers, nil
}
