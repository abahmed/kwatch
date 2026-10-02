package providertest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// Now is the fixed time every recorder dependency reports.
var Now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// Request is one captured HTTP request.
type Request struct {
	Method string
	Path   string
	// RawPath is the path as sent, with its escapes.
	RawPath string
	Query   string
	Header  http.Header
	Body    []byte
}

// JSON decodes the request body into a generic map.
func (r Request) JSON(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("request body is not a JSON object: %v\n%s", err, r.Body)
	}
	return out
}

// Recorder is an httptest server that records every request and answers
// with Reply (200 and "{}" by default).
type Recorder struct {
	Server *httptest.Server
	// Reply writes the response; nil answers 200 with "{}".
	Reply func(w http.ResponseWriter, r Request)

	mu       sync.Mutex
	requests []Request
}

// NewRecorder starts a recorder that is closed when the test ends.
func NewRecorder(t *testing.T) *Recorder {
	t.Helper()
	rec := &Recorder{}
	rec.Server = httptest.NewServer(http.HandlerFunc(rec.serve))
	t.Cleanup(rec.Server.Close)
	return rec
}

func (rec *Recorder) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	req := Request{
		Method: r.Method, Path: r.URL.Path, RawPath: r.URL.EscapedPath(),
		Query:  r.URL.RawQuery,
		Header: r.Header.Clone(), Body: body,
	}
	rec.mu.Lock()
	rec.requests = append(rec.requests, req)
	reply := rec.Reply
	rec.mu.Unlock()
	if reply != nil {
		reply(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("{}"))
}

// URL is the server's base URL.
func (rec *Recorder) URL() string { return rec.Server.URL }

// Requests returns a copy of every captured request, in order.
func (rec *Recorder) Requests() []Request {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]Request(nil), rec.requests...)
}

// Last returns the most recent request and fails when there is none.
func (rec *Recorder) Last(t *testing.T) Request {
	t.Helper()
	requests := rec.Requests()
	if len(requests) == 0 {
		t.Fatal("provider sent no request")
	}
	return requests[len(requests)-1]
}

// Reset forgets captured requests.
func (rec *Recorder) Reset() {
	rec.mu.Lock()
	rec.requests = nil
	rec.mu.Unlock()
}

// Dependencies are provider dependencies that talk to this recorder.
func (rec *Recorder) Dependencies() transport.Dependencies {
	return transport.Dependencies{
		HTTPClient: rec.Server.Client(),
		Clock:      clock.Func(func() time.Time { return Now }),
	}
}
