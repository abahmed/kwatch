package kube

import (
	"context"
	"errors"
	"net"
	"net/http"
)

// errUnrestrictedClient is the error of a public-only HTTP probe whose
// client transport cannot be made to refuse private addresses.
var errUnrestrictedClient = errors.New(
	"blocked: probe client cannot refuse private addresses")

type dialFunc = func(context.Context, string, string) (net.Conn, error)

// publicAccess is how the prober reaches a PublicOnly target.
type publicAccess struct {
	dial dialFunc
	// client is cfg.HTTPClient with connections limited to public
	// addresses; nil when there is no client or its transport is not an
	// *http.Transport, so it cannot be restricted.
	client *http.Client
}

// newPublicAccess builds the restricted dial and HTTP client. base is the
// dialer underneath: nil means one that refuses private addresses itself.
func newPublicAccess(
	resolver Resolver, base dialFunc, client *http.Client,
) publicAccess {
	if resolver == nil {
		resolver = &net.Resolver{}
	}
	if base == nil {
		base = externalDialer()
	}
	dial := publicDial(resolver, base)
	return publicAccess{dial: dial, client: publicClient(client, dial)}
}

// publicDial resolves a name here, refuses it when any answer is private,
// and dials an address it checked, so the name cannot answer differently
// at connect time. The base dialer is the last guard for a literal IP.
func publicDial(resolver Resolver, base dialFunc) dialFunc {
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
			if privateAddress(net.ParseIP(answer)) {
				return nil, errBlockedAddress
			}
		}
		return base(ctx, network, net.JoinHostPort(answers[0], port))
	}
}

// publicClient copies base with a transport that dials through dial. It
// never keeps connections alive, so every probe resolves and checks again,
// and never uses a proxy, which would dial on kwatch's behalf.
func publicClient(base *http.Client, dial dialFunc) *http.Client {
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
