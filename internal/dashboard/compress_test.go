package dashboard

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// zipGet calls the server with Accept-Encoding: gzip and, when tag is
// set, If-None-Match.
func zipGet(
	t *testing.T, s *Server, path, tag string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	if tag != "" {
		req.Header.Set("If-None-Match", tag)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func unzip(t *testing.T, body io.Reader) string {
	t.Helper()
	zr, err := gzip.NewReader(body)
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// openIncidentsForZip is enough issues for a list body worth
// compressing.
func openIncidentsForZip() []incident.Incident {
	items := make([]incident.Incident, 0, 30)
	for i := range 30 {
		items = append(items, testIncident(fmt.Sprintf("inc-%02d", i),
			"shop", time.Duration(-i)*time.Minute))
	}
	return items
}

func TestCompressListKeepsETagAnd304(t *testing.T) {
	s := newTestServer(openIncidentsForZip()...)
	plain := get(t, s, "/api/incidents")
	w := zipGet(t, s, "/api/incidents", "")
	if w.Code != http.StatusOK ||
		w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("status %d, encoding %q", w.Code,
			w.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary = %q", w.Header().Get("Vary"))
	}
	if got := unzip(t, w.Body); got != plain.Body.String() {
		t.Errorf("unzipped body differs from the plain one")
	}
	tag := w.Header().Get("ETag")
	if tag == plain.Header().Get("ETag") || !strings.HasSuffix(tag,
		gzipTagSuffix+`"`) {
		t.Errorf("gzip tag %q, plain %q", tag, plain.Header().Get("ETag"))
	}
	if again := zipGet(t, s, "/api/incidents", tag); again.Code !=
		http.StatusNotModified || again.Header().Get("ETag") != tag {
		t.Errorf("If-None-Match %q: %d %q", tag, again.Code,
			again.Header().Get("ETag"))
	}
	if p := getIfNoneMatch(t, s, "/api/incidents",
		plain.Header().Get("ETag")); p.Code != http.StatusNotModified ||
		p.Header().Get("Content-Encoding") != "" {
		t.Errorf("plain If-None-Match: %d", p.Code)
	}
}

func TestCompressAssetsAndNotTheShortOrUnaccepted(t *testing.T) {
	s := newTestServer()
	shell := zipGet(t, s, "/", "")
	if shell.Header().Get("Content-Encoding") != "gzip" ||
		!strings.Contains(unzip(t, shell.Body), "<!doctype html>") {
		t.Errorf("shell not gzipped")
	}
	if p := get(t, s, "/"); p.Header().Get("Content-Encoding") != "" {
		t.Errorf("gzipped without Accept-Encoding")
	}
	nf := zipGet(t, s, "/api/nope", "")
	if nf.Code != http.StatusNotFound ||
		nf.Header().Get("Content-Encoding") != "" {
		t.Errorf("a short error was compressed: %d", nf.Code)
	}
	if acceptsGzip(&http.Request{Header: http.Header{
		"Accept-Encoding": {"gzip;q=0"}}}) {
		t.Errorf("q=0 accepted")
	}
}

func TestCompressLeavesTheLiveStreamAlone(t *testing.T) {
	s := newTestServer()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Accept-Encoding", "gzip")
	client := &http.Client{Timeout: 5 * time.Second,
		Transport: &http.Transport{DisableCompression: true}}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("Content-Encoding") != "" ||
		!strings.HasPrefix(res.Header.Get("Content-Type"),
			"text/event-stream") {
		t.Errorf("live stream: %q %q", res.Header.Get("Content-Encoding"),
			res.Header.Get("Content-Type"))
	}
}
