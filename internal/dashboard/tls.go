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
		return server.Serve(ln)
	}
	certs := &certReloader{cert: s.opts.TLSCertFile, key: s.opts.TLSKeyFile}
	if _, err := certs.get(nil); err != nil {
		return err
	}
	server.TLSConfig = &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: certs.get,
	}
	return server.ServeTLS(ln, "", "")
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
