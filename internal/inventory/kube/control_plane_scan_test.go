package kube

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apiserverSample is a trimmed /metrics response of a real EKS API
// server (Kubernetes 1.3x): admission webhooks, priority and fairness
// queues, storage and the request histograms of pods.
//
//go:embed testdata/apiserver_metrics_sample.txt
var apiserverSample []byte

func TestScanControlPlaneReadsARealResponse(t *testing.T) {
	r := scanControlPlane(apiserverSample)

	assert.Equal(t, "1791250898.61", r.process)
	assert.Equal(t, 187887616.0, r.dbBytes)
	assert.Equal(t, 1052.0, r.objects["events"])
	assert.Equal(t, 448.0, r.objects["replicasets.apps"])
	assert.Contains(t, r.inqueue, "workload-high")
	assert.Equal(t, 0.0, r.inqueue["workload-high"])
}

func TestScanControlPlaneSplitsWebhookFailures(t *testing.T) {
	r := scanControlPlane(apiserverSample)

	// mpod.kb.io answered 503 twice but is not rejecting: it fails open.
	assert.Equal(t, 2.0, r.counters["webhook/calls/mpod.kb.io"])
	assert.Equal(t, 2.0, r.counters["webhook/open/mpod.kb.io"])
	assert.Zero(t, r.counters["webhook/closed/mpod.kb.io"])
	assert.Equal(t, 21.0, r.counters["webhook/calls/datadog.webhook.probe"])
	assert.Zero(t, r.counters["webhook/open/datadog.webhook.probe"])
}

func TestScanControlPlaneBuildsHistogramsPerWebhookAndVerb(t *testing.T) {
	r := scanControlPlane(apiserverSample)

	probe := r.webhooks.get("datadog.webhook.probe")
	assert.Equal(t, 21.0, probe.count)
	p99, ok := probe.quantile(0.99)
	require.True(t, ok)
	assert.Less(t, p99, 0.5, "the real webhook answers in milliseconds")
	create := r.requests.get("POST pods")
	assert.Greater(t, create.count, 0.0)
	assert.Empty(t, r.requests.get("GET /healthz").bounds,
		"non-resource paths are not timed")
}

func TestRequestKeySkipsStreamsAndNonResourcePaths(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"create", map[string]string{"verb": "POST", "resource": "pods"},
			"POST pods"},
		{"subresource", map[string]string{"verb": "PUT",
			"resource": "pods", "subresource": "status"}, "PUT pods/status"},
		{"watch", map[string]string{"verb": "WATCH", "resource": "pods"}, ""},
		{"exec", map[string]string{"verb": "GET", "resource": "pods",
			"subresource": "exec"}, ""},
		{"healthz", map[string]string{"verb": "GET",
			"subresource": "/healthz"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, requestKey(c.labels))
		})
	}
}
