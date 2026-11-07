package dashboard

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

// maxSchemeHosts bounds how many hosts the scheme log remembers. A
// dashboard is reached under a handful of names; past the bound the
// oldest host is forgotten. Only signed-in requests are recorded, so a
// stranger sending made-up Host headers can neither grow it nor push
// the real names out.
const maxSchemeHosts = 16

// schemeLog remembers, per Host, the scheme signed-in requests used:
// "https" when isHTTPS says so (kwatch's own TLS, or X-Forwarded-Proto
// from a trusted proxy), else "http". "https" sticks: a later plain
// request for the same host does not turn it back to "http". Alert
// links use it when the Ingress alone cannot tell, as when a load
// balancer ends TLS.
type schemeLog struct {
	mu     sync.Mutex
	byHost map[string]string
	order  []string // hosts, oldest first
}

func (l *schemeLog) record(host, scheme string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byHost == nil {
		l.byHost = make(map[string]string, maxSchemeHosts)
	}
	if _, seen := l.byHost[host]; !seen {
		if len(l.order) == maxSchemeHosts {
			delete(l.byHost, l.order[0])
			l.order = l.order[1:]
		}
		l.order = append(l.order, host)
	}
	if l.byHost[host] != "https" {
		l.byHost[host] = scheme
	}
}

func (l *schemeLog) lookup(host string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	scheme, ok := l.byHost[host]
	return scheme, ok
}

// recordScheme notes the scheme of a request under its host before
// passing it on. It wraps handlers behind the auth middleware only.
func (s *Server) recordScheme(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if host := requestHost(r); host != "" {
			scheme := "http"
			if s.auth.isHTTPS(r) {
				scheme = "https"
			}
			s.schemes.record(host, scheme)
		}
		next.ServeHTTP(w, r)
	})
}

// SchemeFor is the scheme ("https" or "http") signed-in requests for
// host came in with, as the sign-in checks judge it: "https" once any
// did. false when no signed-in request for host was seen.
func (s *Server) SchemeFor(host string) (string, bool) {
	return s.schemes.lookup(strings.ToLower(host))
}

// RequiresHTTPS reports whether sign-in refuses plain http (shared
// mode), so a link to the dashboard is only useful over https.
func (s *Server) RequiresHTTPS() bool {
	return s.opts.shared()
}

// requestHost is the request's Host without a port, in lower case.
func requestHost(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}
