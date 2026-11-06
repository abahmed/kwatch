package dashboard

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// rawGet sends one request line to a server on a pageListener and
// returns the whole reply.
func rawGet(t *testing.T, target string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := newTestServer()
	srv := &http.Server{Handler: s.Handler()}
	go func() { _ = srv.Serve(pageListener{ln, s.assets.badRequest}) }()
	t.Cleanup(func() { _ = srv.Close() })
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: x\r\n"+
		"Connection: close\r\n\r\n", target)
	body, _ := io.ReadAll(bufio.NewReader(c))
	return string(body)
}

func TestPageListenerAnswersABrokenEscapeWithThePage(t *testing.T) {
	got := rawGet(t, "/issues/%E0%A4%A")
	if !strings.HasPrefix(got, "HTTP/1.1 400 Bad Request\r\n") ||
		!strings.Contains(got, "text/html") ||
		!strings.Contains(got, "<title>Not found - kwatch</title>") ||
		!strings.Contains(got, "Content-Security-Policy: ") {
		t.Errorf("reply = %q", got)
	}
}

func TestPageListenerLeavesOtherRepliesAlone(t *testing.T) {
	got := rawGet(t, "/nowhere")
	if !strings.HasPrefix(got, "HTTP/1.1 404 Not Found\r\n") ||
		!strings.Contains(got, "Page not found") {
		t.Errorf("reply = %q", got)
	}
}
