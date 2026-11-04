package dashboard

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"strconv"
)

// goBadRequest is what net/http writes on its own, before any handler
// runs, for a request it cannot parse, such as an address with a broken
// escape (/issues/%E0%A4%A): a bare plain-text 400.
const goBadRequest = "HTTP/1.1 400 Bad Request\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"Connection: close\r\n\r\n400 Bad Request"

// badRequestReply is what a browser gets instead: page, the page every
// other unknown address shows, still a 400, with the headers the
// dashboard sends on every page (pageHeaders), and
// Strict-Transport-Security when hsts is set.
func badRequestReply(page []byte, hsts bool) []byte {
	var b bytes.Buffer
	b.WriteString("HTTP/1.1 400 Bad Request\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n")
	if hsts {
		b.WriteString("Strict-Transport-Security: " + hstsValue + "\r\n")
	}
	for _, kv := range pageHeaders {
		b.WriteString(kv[0] + ": " + kv[1] + "\r\n")
	}
	b.WriteString("Content-Length: " + strconv.Itoa(len(page)) + "\r\n" +
		"Connection: close\r\n\r\n")
	b.Write(page)
	return b.Bytes()
}

// pageListener hands out connections that answer an unparsable request
// with reply (badRequestReply). Under TLS that kwatch serves itself it
// wraps the TLS listener, so the bytes it compares are the plaintext
// reply.
type pageListener struct {
	net.Listener
	reply []byte
}

func (l pageListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tc, ok := c.(*tls.Conn); ok {
		return tlsPageConn{pageConn{c, l.reply}, tc}, nil
	}
	return pageConn{c, l.reply}, nil
}

// tlsPageConn keeps the methods net/http looks for on a TLS connection:
// with them it runs the handshake, fills Request.TLS and serves HTTP/2
// when ALPN chose "h2", as it does for a bare *tls.Conn.
type tlsPageConn struct {
	pageConn
	tls *tls.Conn
}

func (c tlsPageConn) ConnectionState() tls.ConnectionState {
	return c.tls.ConnectionState()
}

func (c tlsPageConn) HandshakeContext(ctx context.Context) error {
	return c.tls.HandshakeContext(ctx)
}

// pageConn swaps net/http's own bare 400 for reply. That reply is
// written in one call right before the connection closes, and no
// handler's response is ever exactly those bytes.
type pageConn struct {
	net.Conn
	reply []byte
}

func (c pageConn) Write(p []byte) (int, error) {
	if !bytes.Equal(p, []byte(goBadRequest)) {
		return c.Conn.Write(p)
	}
	if _, err := c.Conn.Write(c.reply); err != nil {
		return 0, err
	}
	return len(p), nil
}

// CloseWrite passes a half-close on, as net/http asks of the
// connections it reads a request body from.
func (c pageConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}
