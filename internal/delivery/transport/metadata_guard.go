package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/abahmed/kwatch/internal/cloudmeta"
)

// ErrMetadataAddress is the error of a send whose destination is a cloud
// instance-metadata address. Such an endpoint hands out the node's cloud
// credentials, so a webhook URL pointing there is refused for good.
var ErrMetadataAddress = errors.New("refused: cloud metadata address")

// guardDialer is the dialer of a guarded transport. Its timeouts are
// those of the application's outbound client (see kubeclient): the
// transport's dial function cannot be inspected, so the guard builds the
// same dialer again, with the metadata check added at connect time.
func guardDialer() *net.Dialer {
	return &net.Dialer{Timeout: 30 * time.Second,
		KeepAlive: 30 * time.Second}
}

// GuardMetadata returns a copy of c whose requests never reach a cloud
// metadata address: a URL naming one is refused before any I/O, and when
// the transport is an *http.Transport its dialer refuses to connect to a
// metadata address. The check runs on the address being connected, after
// the name was resolved, so a name that resolves (or later rebinds) to a
// metadata address is refused too, while the dialer keeps its own address
// order, Happy Eyeballs fallback and timeouts. A connection to the proxy
// is not checked: the operator configured that address. A nil c stays
// nil.
func GuardMetadata(c *http.Client) *http.Client {
	return guardMetadata(c, guardDialer())
}

// guardMetadata is GuardMetadata with base as the dialer to guard; its
// own Control hooks still run, after the metadata check.
func guardMetadata(c *http.Client, base *net.Dialer) *http.Client {
	if c == nil {
		return nil
	}
	rt := c.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	if t, ok := rt.(*http.Transport); ok {
		rt = guardTransport(t, base)
	}
	copied := *c
	copied.Transport = metadataTransport{base: rt}
	return &copied
}

// guardTransport is a clone of t whose dials are checked, except those
// to an address its Proxy function named.
func guardTransport(t *http.Transport, base *net.Dialer) *http.Transport {
	clone := t.Clone()
	proxies := &proxyAddrs{}
	if proxy := clone.Proxy; proxy != nil {
		clone.Proxy = func(r *http.Request) (*url.URL, error) {
			u, err := proxy(r)
			if err == nil && u != nil {
				proxies.note(u)
			}
			return u, err
		}
	}
	direct, guarded := *base, metadataDialer(base)
	clone.DialContext = func(
		ctx context.Context, network, address string,
	) (net.Conn, error) {
		if proxies.has(address) {
			return direct.DialContext(ctx, network, address)
		}
		return guarded.DialContext(ctx, network, address)
	}
	return clone
}

// proxyAddrs are the addresses ("host:port", as the transport dials
// them) of the proxies the transport used. There are as many as the
// operator configured.
type proxyAddrs struct {
	seen sync.Map
}

// proxyPorts are the default ports of the proxy schemes net/http knows.
var proxyPorts = map[string]string{"http": "80", "https": "443",
	"socks5": "1080", "socks5h": "1080"}

func (p *proxyAddrs) note(u *url.URL) {
	port := u.Port()
	if port == "" {
		port = proxyPorts[u.Scheme]
	}
	p.seen.Store(strings.ToLower(net.JoinHostPort(u.Hostname(), port)),
		true)
}

func (p *proxyAddrs) has(address string) bool {
	_, ok := p.seen.Load(strings.ToLower(address))
	return ok
}

// metadataTransport refuses a request whose URL names a metadata host.
// Behind a proxy that is the only check: the proxy resolves the name.
type metadataTransport struct {
	base http.RoundTripper
}

func (t metadataTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if cloudmeta.Host(r.URL.Hostname()) {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, Permanent(ErrMetadataAddress)
	}
	return t.base.RoundTrip(r)
}

type controlFunc = func(context.Context, string, string,
	syscall.RawConn) error

// GuardDialer returns a copy of base that refuses, before connecting, a
// cloud metadata address (ErrMetadataAddress, permanent), checked on the
// resolved address as guarded HTTP clients are. Providers that dial
// another protocol (SMTP) use it; base's own Control hooks run after the
// check, and its timeouts are kept.
func GuardDialer(base *net.Dialer) *net.Dialer {
	return metadataDialer(base)
}

// metadataDialer is a copy of base that refuses, before connecting, an
// address that is a metadata address.
func metadataDialer(base *net.Dialer) *net.Dialer {
	d := *base
	next := base.ControlContext
	if next == nil && base.Control != nil {
		control := base.Control
		next = func(_ context.Context, network, address string,
			c syscall.RawConn) error {
			return control(network, address, c)
		}
	}
	d.Control, d.ControlContext = nil, metadataControl(next)
	return &d
}

func metadataControl(next controlFunc) controlFunc {
	return func(ctx context.Context, network, address string,
		c syscall.RawConn) error {
		if metadataHostPort(address) {
			return Permanent(ErrMetadataAddress)
		}
		if next == nil {
			return nil
		}
		return next(ctx, network, address, c)
	}
}

// metadataHostPort reports a dialed "address:port" whose address is a
// metadata address.
func metadataHostPort(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	return cloudmeta.Host(host)
}
