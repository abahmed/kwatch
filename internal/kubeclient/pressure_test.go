package kubeclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/metrics"
)

// fakeTime is a clock the test moves by hand.
type fakeTime struct{ now time.Time }

func (f *fakeTime) Now() time.Time      { return f.now }
func (f *fakeTime) add(d time.Duration) { f.now = f.now.Add(d) }

func newFakeTime() *fakeTime { return &fakeTime{now: time.Unix(1e9, 0)} }

var _ clock.Clock = (*fakeTime)(nil)

func noteN(p *Pressure, n int, s PressureSignal) {
	for range n {
		p.Note(s)
	}
}

// One slow request is noise; three signs within a minute are pressure,
// and it ends only after two quiet minutes.
func TestPressureEntersOnRepeatedSignsAndLeavesWhenQuiet(t *testing.T) {
	ft := newFakeTime()
	p := NewPressure(ft)

	noteN(p, 2, SignalSlow)
	assert.False(t, p.High(), "two signs are not pressure yet")
	ft.add(30 * time.Second)
	p.Note(SignalThrottled)
	require.True(t, p.High())
	assert.EqualValues(t, 1, metrics.DefaultRegistry().APIPressure.Load())

	ft.add(pressureQuiet - time.Second)
	assert.True(t, p.High(), "not quiet long enough")
	p.Note(SignalLimiter)
	ft.add(pressureQuiet - time.Second)
	assert.True(t, p.High(), "a new sign restarts the quiet")
	ft.add(time.Second)
	assert.False(t, p.High(), "recovers by itself")
	assert.EqualValues(t, 0, metrics.DefaultRegistry().APIPressure.Load())
}

// Signs spread wider than the window never add up to pressure.
func TestPressureIgnoresSpreadOutSigns(t *testing.T) {
	ft := newFakeTime()
	p := NewPressure(ft)
	for range 10 {
		p.Note(SignalSlow)
		ft.add(31 * time.Second)
	}
	assert.False(t, p.High())
}

// A nil Pressure is never high and notes nothing.
func TestNilPressureIsNeverHigh(t *testing.T) {
	var p *Pressure
	p.Note(SignalThrottled)
	assert.False(t, p.High())
}

// The round tripper notes HTTP 429 and slow GETs of one object; a watch
// or a followed log is long by design.
func TestPressureTripperNotesThrottledAndSlowRequests(t *testing.T) {
	ft := newFakeTime()
	p := NewPressure(ft)
	status, delay := http.StatusOK, time.Duration(0)
	rt := p.Wrap(roundTrip(func(*http.Request) (*http.Response, error) {
		ft.add(delay)
		return &http.Response{StatusCode: status}, nil
	}))
	call := func(target string) {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		require.NotNil(t, resp)
	}
	before := metrics.DefaultRegistry().APIPressureSignals[0].Load()

	status = http.StatusTooManyRequests
	call("https://api/api/v1/pods")
	assert.Equal(t, before+1,
		metrics.DefaultRegistry().APIPressureSignals[0].Load())

	status, delay = http.StatusOK, SlowRequest+time.Second
	call("https://api/api/v1/pods?watch=true")
	call("https://api/api/v1/namespaces/a/pods/b/log?follow=true")
	assert.False(t, p.High(), "streams are long by design")
	call("https://api/api/v1/namespaces/a/pods/b")
	call("https://api/apis/apps/v1/namespaces/a/deployments/web")
	assert.True(t, p.High(), "a 429 and two slow GETs by name")
}

// kwatch's own large LISTs at start (and relists), log reads and the
// election's Lease calls take long on a large cluster without any
// pressure: however many there are, they are no sign.
func TestPressureIgnoresSlowListsLogsAndLeases(t *testing.T) {
	ft := newFakeTime()
	p := NewPressure(ft)
	rt := p.Wrap(roundTrip(func(*http.Request) (*http.Response, error) {
		ft.add(SlowRequest + time.Second)
		return &http.Response{StatusCode: http.StatusOK}, nil
	}))
	for _, target := range []string{
		"https://api/api/v1/pods?limit=500",
		"https://api/api/v1/namespaces/a/pods",
		"https://api/apis/apps/v1/replicasets?resourceVersion=0",
		"https://api/api/v1/namespaces/a/pods/b/log?tailLines=100",
		"https://api/api/v1/nodes/n1/proxy/stats/summary",
		"https://api/apis/coordination.k8s.io/v1/namespaces/k/leases/kw",
		"https://api/api", "https://api/apis/apps/v1",
	} {
		for range pressureEnter {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			_, err := rt.RoundTrip(req)
			require.NoError(t, err)
		}
	}
	assert.False(t, p.High())
}

// Only a GET of one object by name is timed.
func TestTimedRequestIsAGetOfOneObject(t *testing.T) {
	cases := map[string]bool{
		"/api/v1/namespaces/a/pods/b":                         true,
		"/api/v1/nodes/n1":                                    true,
		"/api/v1/namespaces/kube-system":                      true,
		"/apis/apps/v1/namespaces/a/deployments/web":          true,
		"/apis/storage.k8s.io/v1/storageclasses/fast":         true,
		"/api/v1/pods":                                        false,
		"/api/v1/namespaces":                                  false,
		"/apis/apps/v1/namespaces/a/deployments":              false,
		"/api/v1/namespaces/a/pods/b/log":                     false,
		"/apis/coordination.k8s.io/v1/namespaces/k/leases/kw": false,
		"/healthz": false,
	}
	for path, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "https://api"+path, nil)
		assert.Equal(t, want, timedRequest(req), path)
	}
	put := httptest.NewRequest(http.MethodPut,
		"https://api/api/v1/namespaces/a/pods/b", nil)
	assert.False(t, timedRequest(put), "only reads are timed")
}

// client-go reports how long its limiter held each request; only a long
// wait is a sign.
func TestPressureObservesClientSideLimiterWaits(t *testing.T) {
	ft := newFakeTime()
	p := NewPressure(ft)
	for range 3 {
		p.Observe(context.Background(), "GET", url.URL{}, LimiterWait)
	}
	assert.False(t, p.High(), "a short wait is normal")
	for range 3 {
		p.Observe(context.Background(), "GET", url.URL{},
			LimiterWait+time.Millisecond)
	}
	assert.True(t, p.High())
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
