package kube

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

var planeStart = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// planeBody renders the metrics of one API server process: calls of
// one verb and resource with a number of fast and slow ones, and the
// webhook calls. Buckets are 0.1s, 1s and 10s.
type planeBody struct {
	process    int
	fast, slow int
	writeFast  int
	writeSlow  int
	hookCalls  int
	hookClosed int
	rejected   int
}

func (b planeBody) text() []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "process_start_time_seconds %d\n", b.process)
	b.request(&out, "GET", "configmaps", b.fast, b.slow)
	b.request(&out, "POST", "pods", b.writeFast, b.writeSlow)
	b.request(&out, "POST", "events", b.writeFast, 0)
	fmt.Fprintf(&out, "apiserver_admission_webhook_request_total"+
		`{code="200",name="opa.example",rejected="false"} %d`+"\n",
		b.hookCalls-b.hookClosed)
	fmt.Fprintf(&out, "apiserver_admission_webhook_request_total"+
		`{code="500",name="opa.example",rejected="true"} %d`+"\n",
		b.hookClosed)
	n := float64(b.hookCalls)
	fmt.Fprintf(&out, "apiserver_admission_webhook_admission_duration_"+
		"seconds_bucket{name=\"opa.example\",le=\"0.1\"} 0\n"+
		"apiserver_admission_webhook_admission_duration_seconds_bucket"+
		"{name=\"opa.example\",le=\"1\"} 0\n"+
		"apiserver_admission_webhook_admission_duration_seconds_bucket"+
		"{name=\"opa.example\",le=\"10\"} %g\n"+
		"apiserver_admission_webhook_admission_duration_seconds_count"+
		"{name=\"opa.example\"} %g\n", n, n)
	fmt.Fprintf(&out, "apiserver_flowcontrol_rejected_requests_total"+
		`{priority_level="workload-low",reason="queue-full"} %d`+"\n",
		b.rejected)
	return []byte(out.String())
}

func (b planeBody) request(
	out *strings.Builder, verb, resource string, fast, slow int,
) {
	labels := fmt.Sprintf(`verb=%q,resource=%q`, verb, resource)
	total := fast + slow
	fmt.Fprintf(out, "apiserver_request_duration_seconds_bucket{%s,"+
		"le=\"0.1\"} %d\napiserver_request_duration_seconds_bucket{%s,"+
		"le=\"1\"} %d\napiserver_request_duration_seconds_bucket{%s,"+
		"le=\"10\"} %d\napiserver_request_duration_seconds_count{%s} %d\n",
		labels, fast, labels, fast, labels, total, labels, total)
}

func readPlane(
	t *testing.T, p *Prober, body planeBody, at time.Time,
) (map[string]inventory.Value, []inventory.Observation) {
	t.Helper()
	attrs := map[string]inventory.Value{}
	obs := p.addControlPlaneMetrics(body.text(), attrs, at)
	return attrs, obs
}

func TestControlPlaneMetricsNeedAPreviousReading(t *testing.T) {
	p := NewProber(ProbeConfig{})

	attrs, obs := readPlane(t, p, planeBody{process: 1, fast: 100}, planeStart)

	assert.NotContains(t, attrs, AttrWritesP99)
	assert.Empty(t, obs)
}

func TestControlPlaneMetricsMeasureOnlyNewCalls(t *testing.T) {
	p := NewProber(ProbeConfig{})
	// Before: a long, fast history. Since then: 100 slow creates.
	readPlane(t, p, planeBody{process: 1, writeFast: 100000}, planeStart)

	attrs, _ := readPlane(t, p,
		planeBody{process: 1, writeFast: 100050, writeSlow: 100},
		planeStart.Add(30*time.Second))

	p99, ok := attrs[AttrWritesP99].AsNumber()
	require.True(t, ok)
	assert.Greater(t, p99, 1000.0, "the history of fast calls is ignored")
	assert.Equal(t, "create pods", attrs[AttrWritesSlowest].AsText())
	calls, _ := attrs[AttrWritesCalls].AsNumber()
	assert.Equal(t, 200.0, calls)
}

func TestControlPlaneMetricsSkipTooFewCalls(t *testing.T) {
	p := NewProber(ProbeConfig{})
	readPlane(t, p, planeBody{process: 1, writeFast: 10}, planeStart)

	attrs, _ := readPlane(t, p,
		planeBody{process: 1, writeFast: 10, writeSlow: 3},
		planeStart.Add(30*time.Second))

	assert.NotContains(t, attrs, AttrWritesP99)
}

func TestControlPlaneMetricsCompareTheSameProcess(t *testing.T) {
	p := NewProber(ProbeConfig{})
	at := planeStart
	next := func(body planeBody) (map[string]inventory.Value, bool) {
		at = at.Add(30 * time.Second)
		attrs, _ := readPlane(t, p, body, at)
		_, ok := attrs[AttrWritesP99]
		return attrs, ok
	}
	_, ok := next(planeBody{process: 1, writeFast: 1000})
	assert.False(t, ok, "first sample of server A")
	_, ok = next(planeBody{process: 2, writeFast: 900000})
	assert.False(t, ok, "first sample of server B is no delta against A")
	_, ok = next(planeBody{process: 1, writeFast: 1100, writeSlow: 1})
	assert.True(t, ok, "A against its own earlier sample")
	_, ok = next(planeBody{process: 2, writeFast: 900100})
	assert.True(t, ok)
}

func TestControlPlaneMetricsSkipARestartedServer(t *testing.T) {
	p := NewProber(ProbeConfig{})
	readPlane(t, p, planeBody{process: 1, writeFast: 5000}, planeStart)

	// Same process id, lower counters: it cannot be the same process.
	attrs, _ := readPlane(t, p, planeBody{process: 1, writeFast: 100},
		planeStart.Add(30*time.Second))

	assert.NotContains(t, attrs, AttrWritesP99)
}

func TestControlPlaneMetricsRecordThrottling(t *testing.T) {
	p := NewProber(ProbeConfig{})
	readPlane(t, p, planeBody{process: 1}, planeStart)

	attrs, _ := readPlane(t, p, planeBody{process: 1, rejected: 60},
		planeStart.Add(30*time.Second))

	rate, _ := attrs[AttrThrottledRate].AsNumber()
	assert.InDelta(t, 2.0, rate, 0.001)
	assert.Equal(t, "workload-low", attrs[AttrThrottledLevel].AsText())
}

func TestControlPlaneMetricsRecordStorageFromOneReading(t *testing.T) {
	p := NewProber(ProbeConfig{})
	attrs := map[string]inventory.Value{}

	p.addControlPlaneMetrics(apiserverSample, attrs, planeStart)

	assert.Equal(t, "events", attrs[AttrObjectsResource].AsText())
	count, _ := attrs[AttrObjectsCount].AsNumber()
	assert.Equal(t, 1052.0, count)
	size, _ := attrs[AttrEtcdDBBytes].AsNumber()
	assert.Equal(t, 187887616.0, size)
}
