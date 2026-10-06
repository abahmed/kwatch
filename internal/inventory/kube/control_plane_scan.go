package kube

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// Metric families of the API server that describe its own health.
const (
	metricRequestDuration = "apiserver_request_duration_seconds"
	metricEtcdDuration    = "etcd_request_duration_seconds"
	metricWebhookDuration = "apiserver_admission_webhook_" +
		"admission_duration_seconds"
	metricWebhookRequests = "apiserver_admission_webhook_request_total"
	metricAPFRejected     = "apiserver_flowcontrol_rejected_requests_total"
	metricAPFInqueue      = "apiserver_flowcontrol_current_inqueue_requests"
	metricStorageSize     = "apiserver_storage_size_bytes"
	metricStorageDBSize   = "apiserver_storage_db_total_size_in_bytes"
	metricEtcdDBSize      = "etcd_db_total_size_in_bytes"
	metricStorageObjects  = "apiserver_storage_objects"
	metricProcessStart    = "process_start_time_seconds"
)

// planeReading is everything of interest in one /metrics response.
type planeReading struct {
	// process tells API server processes apart; empty when unknown.
	process string
	// requests are keyed by verb and resource, see requestKey.
	requests histogramReading
	etcd     histogramReading
	// webhooks are keyed by webhook name.
	webhooks histogramReading
	// counters are monotonic counts, keyed by counter name and label.
	counters map[string]float64
	// inqueue is the requests waiting now, by priority level.
	inqueue map[string]float64
	// dbBytes is the largest etcd database size reported.
	dbBytes float64
	// objects is the stored object count by resource.
	objects map[string]float64
}

// planeFamilies are the metric names the scan reads; a line that does
// not start with one is skipped before it is parsed.
var planeFamilies = []string{
	metricRequestDuration, metricEtcdDuration, metricWebhookDuration,
	metricWebhookRequests, metricAPFRejected, metricAPFInqueue,
	metricStorageSize, metricStorageDBSize, metricEtcdDBSize,
	metricStorageObjects, metricProcessStart,
}

// scanControlPlane reads the control-plane metrics of one response in a
// single pass.
func scanControlPlane(body []byte) *planeReading {
	r := &planeReading{
		requests: histogramReading{}, etcd: histogramReading{},
		webhooks: histogramReading{}, counters: map[string]float64{},
		inqueue: map[string]float64{}, objects: map[string]float64{},
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !hasFamily(line) {
			continue
		}
		if name, labels, value, ok := metricLine(line); ok {
			r.add(name, labels, value)
		}
	}
	return r
}

func hasFamily(line string) bool {
	for _, family := range planeFamilies {
		if strings.HasPrefix(line, family) {
			return true
		}
	}
	return false
}

// add files one sample under the family it belongs to.
func (r *planeReading) add(
	name string, labels map[string]string, value float64,
) {
	family, suffix := splitHistogramName(name)
	switch family {
	case metricRequestDuration:
		r.requests.add(requestKey(labels), suffix, labels["le"], value)
	case metricEtcdDuration:
		r.etcd.add("etcd", suffix, labels["le"], value)
	case metricWebhookDuration:
		r.webhooks.add(labels["name"], suffix, labels["le"], value)
	case metricWebhookRequests:
		r.addWebhookRequest(labels, value)
	case metricAPFRejected:
		r.counters["apf/"+labels["priority_level"]] += value
	case metricAPFInqueue:
		r.inqueue[labels["priority_level"]] += value
	case metricStorageSize, metricStorageDBSize, metricEtcdDBSize:
		r.dbBytes = max(r.dbBytes, value)
	case metricStorageObjects:
		r.objects[labels["resource"]] += value
	case metricProcessStart:
		r.process = strconv.FormatFloat(value, 'f', -1, 64)
	}
}

// splitHistogramName separates the histogram family from the _bucket,
// _sum or _count suffix; other names are returned whole.
func splitHistogramName(name string) (family, suffix string) {
	for _, s := range []string{"_bucket", "_sum", "_count"} {
		if base, ok := strings.CutSuffix(name, s); ok {
			return base, s
		}
	}
	return name, ""
}

// addWebhookRequest counts the calls to one webhook, and the failed
// ones. A call that got no usable answer (any status but 2xx) was
// rejected when the webhook fails closed and let through when it fails
// open; the API server's "rejected" label tells which.
func (r *planeReading) addWebhookRequest(
	labels map[string]string, value float64,
) {
	name := labels["name"]
	r.counters["webhook/calls/"+name] += value
	if strings.HasPrefix(labels["code"], "2") {
		return
	}
	if labels["rejected"] == "true" {
		r.counters["webhook/closed/"+name] += value
	} else {
		r.counters["webhook/open/"+name] += value
	}
}

// ignoredSubresources are streams that stay open as long as someone
// uses them, so their duration says nothing about the API server.
var ignoredSubresources = map[string]bool{
	"exec": true, "attach": true, "portforward": true, "proxy": true,
	"log": true,
}

// requestKey is "VERB resource" for the requests worth timing, and empty
// for watches, streams and the non-resource paths such as /healthz.
func requestKey(labels map[string]string) string {
	verb, resource := labels["verb"], labels["resource"]
	switch verb {
	case "WATCH", "WATCHLIST", "CONNECT", "":
		return ""
	}
	if resource == "" || ignoredSubresources[labels["subresource"]] {
		return ""
	}
	if sub := labels["subresource"]; sub != "" {
		resource += "/" + sub
	}
	return verb + " " + resource
}
