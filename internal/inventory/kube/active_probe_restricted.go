package kube

import (
	"context"
	"errors"
	"net"
	"net/http"
)

// errUnrestrictableClient is the error of a restricted HTTP probe whose
// client transport cannot be made to refuse sensitive addresses.
var errUnrestrictableClient = errors.New(
	"blocked: probe client cannot refuse sensitive addresses")

type dialFunc = func(context.Context, string, string) (net.Conn, error)

// restrictedAccess is how the prober reaches a Restricted target: any
// address except the sensitive ones (SensitiveProbeAddress).
type restrictedAccess struct {
	dial dialFunc
	// client is cfg.HTTPClient with connections that refuse sensitive
	// addresses; nil when there is no client or its transport is not an
	// *http.Transport, so it cannot be restricted.
	client *http.Client
}

// newRestrictedAccess builds the restricted dial and HTTP client. base is the
// dialer underneath: nil means a plain one that also refuses sensitive
// addresses itself.
func newRestrictedAccess(
	resolver Resolver, base dialFunc, client *http.Client,
) restrictedAccess {
	if resolver == nil {
		resolver = &net.Resolver{}
	}
	if base == nil {
		base = (&net.Dialer{Control: refuseSensitive}).DialContext
	}
	dial := restrictedDial(resolver, base)
	return restrictedAccess{dial: dial, client: restrictedClient(client, dial)}
}

// restrictedDial resolves a name here, refuses it when any answer is sensitive,
// and dials an address it checked, so the name cannot answer differently
// at connect time. The base dialer is the last guard for a literal IP.
func restrictedDial(resolver Resolver, base dialFunc) dialFunc {
	return func(
		ctx context.Context, network, address string,
	) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errBlockedAddress
		}
		answers := []string{host}
		if net.ParseIP(host) == nil {
			if answers, err = resolver.LookupHost(ctx, host); err != nil {
				return nil, err
			}
		}
		if len(answers) == 0 {
			return nil, errBlockedAddress
		}
		for _, answer := range answers {
			if SensitiveProbeAddress(net.ParseIP(answer)) {
				return nil, errBlockedAddress
			}
		}
		return base(ctx, network, net.JoinHostPort(answers[0], port))
	}
}

// restrictedClient copies base with a transport that dials through dial. It
// never keeps connections alive, so every probe resolves and checks again,
// and never uses a proxy, which would dial on kwatch's behalf.
func restrictedClient(base *http.Client, dial dialFunc) *http.Client {
	if base == nil {
		return nil
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	transport, ok := rt.(*http.Transport)
	if !ok {
		return nil
	}
	clone := transport.Clone()
	clone.DialContext = dial
	clone.DialTLSContext = nil
	clone.Proxy = nil
	clone.DisableKeepAlives = true
	out := *base
	out.Transport = clone
	return &out
}
