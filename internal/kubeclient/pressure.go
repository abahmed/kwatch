package kubeclient

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/metrics"
)

// PressureSignal is one sign that the API server is under pressure.
type PressureSignal int

// The signals, in the order of metrics.APIPressureSignals.
const (
	// SignalThrottled is an HTTP 429 from the API server: its priority
	// and fairness turned the request away.
	SignalThrottled PressureSignal = iota
	// SignalSlow is a GET of one object by name that took longer than
	// SlowRequest (see timedRequest).
	SignalSlow
	// SignalLimiter is a request kwatch's own client-side limiter held
	// back for longer than LimiterWait: kwatch asks more than it may.
	SignalLimiter
)

const (
	// SlowRequest is how long a GET of one object by name may take
	// before it counts as a sign of pressure.
	SlowRequest = 5 * time.Second
	// LimiterWait is how long the client-side limiter may hold a
	// request before that counts as a sign of pressure.
	LimiterWait = time.Second
	// pressureWindow and pressureEnter: this many signs within the
	// window turn the pressure on. One slow request is noise.
	pressureWindow = time.Minute
	pressureEnter  = 3
	// pressureQuiet is how long without any sign turns it off again;
	// longer than the window, so it does not flap on and off.
	pressureQuiet = 2 * time.Minute
)

// Pressure is a small, bounded signal of API server pressure, fed by the
// API clients' responses and client-side limiter waits. While it is high,
// optional work (log sampling, rediscovery of optional kinds) waits;
// detection of core kinds, delivery and state saves never do. It keeps the
// last pressureEnter signs only. It is safe for concurrent use; a nil
// Pressure is never high.
type Pressure struct {
	clock clock.Clock

	mu sync.Mutex
	// recent holds the times of the latest signs, a ring of
	// pressureEnter entries; next is the slot to write.
	recent [pressureEnter]time.Time
	next   int
	last   time.Time
	high   bool
}

// NewPressure returns a pressure signal reading time from c.
func NewPressure(c clock.Clock) *Pressure {
	clock.Require(c)
	return &Pressure{clock: c}
}

// Note records one sign of pressure.
func (p *Pressure) Note(signal PressureSignal) {
	if p == nil {
		return
	}
	metrics.DefaultRegistry().IncAPIPressureSignal(int(signal))
	now := p.clock.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recent[p.next] = now
	p.next = (p.next + 1) % pressureEnter
	p.last = now
	oldest := p.recent[p.next]
	if !p.high && !oldest.IsZero() &&
		now.Sub(oldest) <= pressureWindow {
		p.setLocked(true)
	}
}

// High reports whether the API server is under pressure now. Pressure
// ends after pressureQuiet without a sign.
func (p *Pressure) High() bool {
	if p == nil {
		return false
	}
	now := p.clock.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.high && now.Sub(p.last) >= pressureQuiet {
		p.setLocked(false)
	}
	return p.high
}

func (p *Pressure) setLocked(high bool) {
	p.high = high
	var gauge int64
	if high {
		gauge = 1
		// The ring starts over, so leaving and entering again both need
		// fresh signs.
		p.recent = [pressureEnter]time.Time{}
	}
	metrics.DefaultRegistry().APIPressure.Store(gauge)
	klog.InfoS("kubeclient: API server pressure", "component", "kubeclient",
		"high", high)
}

// Wrap returns rt with throttled and slow answers noted. It is what
// rest.Config.Wrap takes; the response goes back untouched.
func (p *Pressure) Wrap(rt http.RoundTripper) http.RoundTripper {
	return pressureTripper{next: rt, pressure: p}
}

type pressureTripper struct {
	next     http.RoundTripper
	pressure *Pressure
}

// RoundTrip implements http.RoundTripper.
func (t pressureTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	start := t.pressure.clock.Now()
	resp, err := t.next.RoundTrip(r)
	switch {
	case resp != nil && resp.StatusCode == http.StatusTooManyRequests:
		t.pressure.Note(SignalThrottled)
	case err == nil && timedRequest(r) &&
		t.pressure.clock.Now().Sub(start) > SlowRequest:
		t.pressure.Note(SignalSlow)
	}
	return resp, err
}

// timedRequest reports a request whose duration says whether the API
// server is slow: a GET of one object by name, which is small whatever
// the cluster's size. Everything else is left out: a LIST (kwatch's own
// initial lists and relists take long on a large cluster, and run
// exactly while restored incidents rebuild their evidence), a watch,
// a log read or any other subresource (its size is the log's), and the
// Lease calls of the election, whose timing is the election's.
func timedRequest(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL == nil {
		return false
	}
	q := r.URL.Query()
	if q.Has("watch") || q.Has("follow") {
		return false
	}
	resource, named := objectPath(r.URL.Path)
	return named && resource != "leases"
}

// objectPath reads a Kubernetes API path: the resource, and named when
// the path ends at one object (/api/v1/namespaces/a/pods/b, not the
// collection /api/v1/pods nor a subresource .../pods/b/log).
func objectPath(path string) (resource string, named bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case len(parts) > 2 && parts[0] == "api":
		parts = parts[2:]
	case len(parts) > 3 && parts[0] == "apis":
		parts = parts[3:]
	default:
		return "", false
	}
	if len(parts) > 2 && parts[0] == "namespaces" {
		parts = parts[2:]
	}
	if len(parts) != 2 || parts[1] == "" {
		return "", false
	}
	return parts[0], true
}

// Observe implements client-go's LatencyMetric for the client-side rate
// limiter (metrics.RegisterOpts.RateLimiterLatency).
func (p *Pressure) Observe(
	_ context.Context, _ string, _ url.URL, latency time.Duration,
) {
	if latency > LimiterWait {
		p.Note(SignalLimiter)
	}
}
