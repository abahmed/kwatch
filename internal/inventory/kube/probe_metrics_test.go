package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
