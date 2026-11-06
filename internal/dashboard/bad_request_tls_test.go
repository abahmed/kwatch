package dashboard

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serveTLS runs a dashboard over kwatch's own TLS on a free port and
// returns its address and the certificate directory.
func serveTLS(t *testing.T) (string, string) {
	t.Helper()
	return serveTLSMode(t, false)
}

// serveTLSMode is serveTLS, in shared mode when shared is set.
func serveTLSMode(t *testing.T, shared bool) (string, string) {
	t.Helper()
	dir := t.TempDir()
	writePair(t, dir, "first", testEpoch)
	s := newTestServer()
	s.opts.Shared = shared
	s.opts.TLSCertFile = filepath.Join(dir, "tls.crt")
	s.opts.TLSKeyFile = filepath.Join(dir, "tls.key")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: s.Handler()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.serve(srv, ln)
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		<-done
	})
	return ln.Addr().String(), dir
}

func insecureTLS(protos ...string) *tls.Config {
	return &tls.Config{InsecureSkipVerify: true, NextProtos: protos,
		MinVersion: tls.VersionTLS12}
}

func TestTLSServeAnswersABrokenEscapeWithThePage(t *testing.T) {
	addr, _ := serveTLS(t)
	c, err := tls.Dial("tcp", addr, insecureTLS("http/1.1"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = fmt.Fprintf(c, "GET /issues/%%E0%%A4%%A HTTP/1.1\r\nHost: x\r\n"+
		"Connection: close\r\n\r\n")
	body, _ := io.ReadAll(bufio.NewReader(c))
	got := string(body)
	if !strings.HasPrefix(got, "HTTP/1.1 400 Bad Request\r\n") ||
		!strings.Contains(got, "text/html") ||
		!strings.Contains(got, "<title>Not found - kwatch</title>") ||
		!strings.Contains(got, "Content-Security-Policy: ") {
		t.Errorf("reply = %q", got)
	}
}

func tlsGet(t *testing.T, addr string, h2 bool) *http.Response {
	t.Helper()
	tr := &http.Transport{TLSClientConfig: insecureTLS(),
		ForceAttemptHTTP2: h2}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get("https://" + addr + "/nowhere")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

func TestTLSServeNegotiatesHTTP2(t *testing.T) {
	addr, _ := serveTLS(t)
	resp := tlsGet(t, addr, true)
	if resp.ProtoMajor != 2 || resp.StatusCode != http.StatusNotFound {
		t.Errorf("proto = %s, status = %d", resp.Proto, resp.StatusCode)
	}
	if resp.TLS == nil || resp.TLS.NegotiatedProtocol != "h2" {
		t.Errorf("ALPN = %+v", resp.TLS)
	}
	resp = tlsGet(t, addr, false)
	if resp.ProtoMajor != 1 || resp.StatusCode != http.StatusNotFound {
		t.Errorf("proto = %s, status = %d", resp.Proto, resp.StatusCode)
	}
}

func TestTLSServePicksUpARenewedCertificate(t *testing.T) {
	addr, dir := serveTLS(t)
	if cn := peerName(t, addr); cn != "first" {
		t.Fatalf("served %q, want first", cn)
	}
	writePair(t, dir, "second", testEpoch.Add(time.Hour))
	if cn := peerName(t, addr); cn != "second" {
		t.Errorf("served %q after renewal, want second", cn)
	}
}

func peerName(t *testing.T, addr string) string {
	t.Helper()
	c, err := tls.Dial("tcp", addr, insecureTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.ConnectionState().PeerCertificates[0].Subject.CommonName
}

// tlsDial opens an HTTP/1.1 TLS connection to addr.
func tlsDial(addr string) (*tls.Conn, error) {
	return tls.Dial("tcp", addr, insecureTLS("http/1.1"))
}
