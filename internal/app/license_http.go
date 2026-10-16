package app

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/metrics"
)

// renewTracker counts activation and renewal attempts and remembers when
// the last one succeeded and when the next is due. It watches the license
// HTTP calls instead of changing the manager.
type renewTracker struct {
	now func() time.Time

	mu          sync.Mutex
	lastRenew   time.Time
	nextAttempt time.Time
}

func (t *renewTracker) snapshot() (last, next time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastRenew, t.nextAttempt
}

func (t *renewTracker) scheduled(d time.Duration) {
	t.mu.Lock()
	t.nextAttempt = t.now().Add(d)
	t.mu.Unlock()
}

func (t *renewTracker) record(result string) {
	metrics.DefaultRegistry().IncLicenseRenewal(result)
	if result != "success" {
		return
	}
	t.mu.Lock()
	t.lastRenew = t.now()
	t.mu.Unlock()
}

// trackingTransport classifies activate and renew responses.
type trackingTransport struct {
	next    http.RoundTripper
	tracker *renewTracker
}

func (t trackingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(r)
	path := r.URL.Path
	if !strings.HasSuffix(path, "/activate") &&
		!strings.HasSuffix(path, "/renew") {
		return resp, err
	}
	switch {
	case err != nil:
		t.tracker.record("error")
	case resp.StatusCode == http.StatusOK:
		t.tracker.record("success")
	case resp.StatusCode >= 500 ||
		resp.StatusCode == http.StatusTooManyRequests:
		t.tracker.record("error")
	default:
		t.tracker.record("rejected")
	}
	return resp, err
}

// licenseHTTPClient returns base with the tracker around its transport.
// base carries the proxy and CA settings of the other outbound calls.
func licenseHTTPClient(base *http.Client, tracker *renewTracker) *http.Client {
	var client http.Client
	if base != nil {
		client = *base
	}
	next := client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	client.Transport = trackingTransport{next: next, tracker: tracker}
	return &client
}
