package dashboard

import (
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// serve runs the listener, over TLS when certificate files are set.
func (s *Server) serve(server *http.Server, ln net.Listener) error {
	if s.opts.TLSCertFile == "" {
		return server.Serve(pageListener{ln, s.assets.badRequest})
	}
	certs := &certReloader{cert: s.opts.TLSCertFile, key: s.opts.TLSKeyFile}
	if _, err := certs.get(nil); err != nil {
		return err
	}
	server.TLSConfig = &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: certs.get,
		// ServeTLS would add these itself. Serve configures HTTP/2 only
		// when "h2" is listed here.
		NextProtos: []string{"h2", "http/1.1"},
	}
	// TLS sits below pageListener, so a pageConn sees the plaintext
	// reply and can swap net/http's bare 400 for the page. Shared mode
	// over kwatch's own TLS sends HSTS on every page (sendHSTS), so the
	// 400 does too.
	tlsLn := tls.NewListener(ln, server.TLSConfig)
	reply := s.assets.badRequest
	if s.opts.shared() {
		reply = badRequestReply(s.assets.notFound, true)
	}
	return server.Serve(pageListener{tlsLn, reply})
}

// checkCertificate reports a TLS key pair that cannot be loaded, before
// the port is bound. Without TLS there is nothing to check.
func (s *Server) checkCertificate() error {
	if s.opts.TLSCertFile == "" {
		return nil
	}
	_, err := tls.LoadX509KeyPair(s.opts.TLSCertFile, s.opts.TLSKeyFile)
	return err
}

// failed closes the listener of a Serve that ended with err, so the next
// Open binds the port again with a new server: a listener that failed
// cannot be served a second time. After Stop there is nothing to reopen.
func (s *Server) failed(
	server *http.Server, listener net.Listener, err error,
) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.stopErr = err
	if s.stopped || s.server != server {
		return
	}
	_ = listener.Close()
	s.server, s.listener, s.started = nil, nil, false
}

// certReloader loads the key pair again when either file changes, so a
// certificate renewed in its Secret is served without a restart.
type certReloader struct {
	cert, key string

	mu      sync.Mutex
	loaded  *tls.Certificate
	modTime time.Time
}

func (c *certReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	newest, err := c.newest()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded != nil && !newest.After(c.modTime) {
		return c.loaded, nil
	}
	pair, err := tls.LoadX509KeyPair(c.cert, c.key)
	if err != nil {
		if c.loaded != nil {
			// A half-written renewal: keep serving the last good pair.
			return c.loaded, nil
		}
		return nil, err
	}
	c.loaded, c.modTime = &pair, newest
	return c.loaded, nil
}

func (c *certReloader) newest() (time.Time, error) {
	var newest time.Time
	for _, name := range []string{c.cert, c.key} {
		info, err := os.Stat(name)
		if err != nil {
			return time.Time{}, err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, nil
}
