package kube

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestRequestCountsSplitServerErrors(t *testing.T) {
	body := []byte(`# HELP apiserver_request_total Counter of requests.
apiserver_request_total{code="200",verb="GET"} 900
apiserver_request_total{code="404",verb="GET"} 50
apiserver_request_total{code="500",verb="LIST"} 30
apiserver_request_total{code="503",verb="GET"} 20
apiserver_request_duration_seconds_count{verb="GET"} 950
`)

	total, errors := requestCounts(body)

	assert.Equal(t, 1000.0, total)
	assert.Equal(t, 50.0, errors)
}

func TestForEachMetricVisitsOnlyTheNamedSeries(t *testing.T) {
	body := []byte(`coredns_dns_responses_total{rcode="NOERROR"} 400
coredns_dns_responses_total{rcode="SERVFAIL"} 40
coredns_forward_responses_total{rcode="SERVFAIL"} 40
`)
	var total, servfail float64
	forEachMetric(body, "coredns_dns_responses_total", func(
		labels map[string]string, value float64,
	) {
		total += value
		if labels["rcode"] == "SERVFAIL" {
			servfail += value
		}
	})
	assert.Equal(t, 440.0, total)
	assert.Equal(t, 40.0, servfail)
}

func apiBody(start, requests int) []byte {
	return []byte(fmt.Sprintf("process_start_time_seconds %d\n"+
		"apiserver_request_total{code=\"200\"} %d\n", start, requests))
}

func TestAPIRatesNeedTwoSamplesOfTheSameServer(t *testing.T) {
	p := NewProber(ProbeConfig{})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rate := func(body []byte) (float64, bool) {
		attrs := map[string]inventory.Value{}
		now = now.Add(30 * time.Second)
		p.serverRates(body, attrs, now)
		value, ok := attrs[AttrAPIRequestRate]
		if !ok {
			return 0, false
		}
		return value.AsNumber()
	}
	// Two API servers answer in turn; their counters differ hugely.
	_, ok := rate(apiBody(100, 1000))
	assert.False(t, ok, "first sample of server A")
	_, ok = rate(apiBody(200, 900000))
	assert.False(t, ok, "first sample of server B is no rate against A")
	got, ok := rate(apiBody(100, 1600))
	require.True(t, ok)
	assert.InDelta(t, 600.0/60, got, 0.001, "A against its own sample")
	got, ok = rate(apiBody(200, 900600))
	require.True(t, ok)
	assert.InDelta(t, 600.0/60, got, 0.001)
}

func TestAPIRatesNeedAProcessIdentity(t *testing.T) {
	p := NewProber(ProbeConfig{})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, requests := range []int{10, 20, 30} {
		attrs := map[string]inventory.Value{}
		now = now.Add(30 * time.Second)
		p.serverRates([]byte(fmt.Sprintf(
			"apiserver_request_total{code=\"200\"} %d\n", requests)), attrs, now)
		assert.Empty(t, attrs)
	}
}

func dnsPage(total int) []byte {
	return []byte(fmt.Sprintf(
		"coredns_dns_responses_total{rcode=\"NOERROR\"} %d\n", total))
}

func TestDNSRatesResetWhenThePodSetChanges(t *testing.T) {
	p := NewProber(ProbeConfig{})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rate := func(pages map[string][]byte) (float64, bool) {
		attrs := map[string]inventory.Value{}
		now = now.Add(30 * time.Second)
		p.dnsRates(pages, attrs, now)
		value, ok := attrs[AttrDNSRequestRate]
		if !ok {
			return 0, false
		}
		return value.AsNumber()
	}
	rate(map[string][]byte{"10.0.0.1": dnsPage(1000)})
	got, ok := rate(map[string][]byte{"10.0.0.1": dnsPage(1300)})
	require.True(t, ok)
	assert.InDelta(t, 10.0, got, 0.001)

	// A second DNS pod appears: its whole counter is not a rate.
	_, ok = rate(map[string][]byte{
		"10.0.0.1": dnsPage(1600), "10.0.0.2": dnsPage(50000)})
	assert.False(t, ok, "no summed jump when the pod set changes")
	got, ok = rate(map[string][]byte{
		"10.0.0.1": dnsPage(1900), "10.0.0.2": dnsPage(50300)})
	require.True(t, ok)
	assert.InDelta(t, 20.0, got, 0.001)
}

func TestMetricLineIgnoresOpenMetricsExemplars(t *testing.T) {
	name, labels, value, ok := metricLine(
		`http_requests_total{code="200"} 17 # {trace_id="abc"} 1 1.5`)
	require.True(t, ok)
	assert.Equal(t, "http_requests_total", name)
	assert.Equal(t, map[string]string{"code": "200"}, labels)
	assert.Equal(t, 17.0, value)
}

func TestThrottleRatiosRestartWhenTheLiveCgroupChanges(t *testing.T) {
	c := newCounterRates()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	page := func(id string, throttled, periods int) []byte {
		return []byte(fmt.Sprintf(
			"container_cpu_cfs_throttled_periods_total{namespace=\"n\","+
				"pod=\"p\",container=\"c\",id=\"%s\"} %d\n"+
				"container_cpu_cfs_periods_total{namespace=\"n\","+
				"pod=\"p\",container=\"c\",id=\"%s\"} %d\n",
			id, throttled, id, periods))
	}
	c.throttleRatios(page("old", 100, 1000), now)
	got := c.throttleRatios(page("new", 5000, 9000), now.Add(time.Minute))
	assert.Empty(t, got, "a different cgroup is not comparable")
	got = c.throttleRatios(page("new", 5100, 9200), now.Add(2*time.Minute))
	assert.InDelta(t, 50.0, got["n/p/c"], 0.001)
}
