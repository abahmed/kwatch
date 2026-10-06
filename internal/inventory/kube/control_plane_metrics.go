package kube

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes of the API server entity, read from its own metrics. Each
// describes the calls made since the previous round, not the whole life
// of the process.
const (
	// AttrWritesP99 is the 99th percentile of write calls, in
	// milliseconds, and AttrWritesCalls how many calls it covers.
	AttrWritesP99   = "api.writes.p99.ms"
	AttrWritesCalls = "api.writes.calls"
	// AttrWritesSlowest names the slowest verb and resource of the
	// writes, such as "create pods".
	AttrWritesSlowest = "api.writes.slowest"
	AttrReadsP99      = "api.reads.p99.ms"
	AttrReadsCalls    = "api.reads.calls"
	AttrReadsSlowest  = "api.reads.slowest"
	// AttrEtcdP99 is the 99th percentile of the API server's calls to
	// etcd, in milliseconds.
	AttrEtcdP99 = "etcd.p99.ms"
	// AttrThrottledRate is requests rejected by priority and fairness
	// per second, and AttrThrottledLevel the priority level that
	// rejected the most.
	AttrThrottledRate  = "api.throttled.per.second"
	AttrThrottledLevel = "api.throttled.level"
	// AttrQueuedRequests is how many requests wait in priority and
	// fairness queues now.
	AttrQueuedRequests = "api.queued.requests"
	// AttrEtcdDBBytes is the size of the etcd database.
	AttrEtcdDBBytes = "etcd.db.bytes"
	// AttrObjectsResource and AttrObjectsCount are the resource that has
	// the most objects stored, and how many.
	AttrObjectsResource = "storage.objects.resource"
	AttrObjectsCount    = "storage.objects.count"
)

// Sample sizes below which a percentile is not reported: one slow call
// out of a handful is not a pattern.
const (
	minClassCalls    = 20
	minResourceCalls = 5
)

// writeVerbs are the verbs that change objects; the rest of the timed
// verbs read them.
var writeVerbs = map[string]bool{
	"POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	"DELETECOLLECTION": true,
}

// verbNames say a verb the way kubectl users do.
var verbNames = map[string]string{
	"POST": "create", "PUT": "update", "PATCH": "patch",
	"DELETE": "delete", "DELETECOLLECTION": "deletecollection",
	"GET": "get", "LIST": "list",
}

// planeSample is one reading of a process, kept to compare the next
// reading of the same process with.
type planeSample struct {
	at      time.Time
	reading *planeReading
}

// planeMemory remembers the latest reading of each API server process.
// Several may answer one Service in turn, so a reading is compared with
// the previous reading of the same process, which can be a few rounds
// back; one that identifies no process is never compared.
type planeMemory struct {
	byProcess map[string]planeSample
}

// compare stores reading and returns the previous reading of the same
// process, or false when there is none or it is too old to describe the
// round before.
func (m *planeMemory) compare(
	reading *planeReading, now time.Time,
) (planeSample, bool) {
	if m.byProcess == nil {
		m.byProcess = map[string]planeSample{}
	}
	for process, sample := range m.byProcess {
		if now.Sub(sample.at) > metricsKeepFor {
			delete(m.byProcess, process)
		}
	}
	if reading.process == "" {
		return planeSample{}, false
	}
	previous, ok := m.byProcess[reading.process]
	m.byProcess[reading.process] = planeSample{at: now, reading: reading}
	return previous, ok
}

// latency is the percentile of one class of calls and its slowest part.
type latency struct {
	p99     float64
	calls   float64
	slowest string
}

// classLatencies computes the write and read latencies of the calls made
// between two readings.
func classLatencies(
	now, before *planeReading,
) (writes, reads latency) {
	var writeAll, readAll histogram
	var worstWrite, worstRead rankedCall
	keys := sortedNames(now.requests)
	for _, key := range keys {
		delta, ok := deltaOf(now.requests.get(key), before.requests.get(key))
		if !ok {
			continue
		}
		verb, resource, _ := strings.Cut(key, " ")
		call := rankedCall{label: verbNames[verb] + " " + resource}
		if writeVerbs[verb] {
			writeAll = writeAll.merge(delta)
			worstWrite = worstWrite.consider(call, delta)
		} else {
			readAll = readAll.merge(delta)
			worstRead = worstRead.consider(call, delta)
		}
	}
	writes = summarize(writeAll, worstWrite)
	reads = summarize(readAll, worstRead)
	return writes, reads
}

// deltaOf is the calls made since before; a series that did not exist
// before counts from zero.
func deltaOf(now, before histogram) (histogram, bool) {
	if len(before.bounds) == 0 {
		return now, true
	}
	return now.since(before)
}

// rankedCall is the slowest verb and resource seen so far.
type rankedCall struct {
	label string
	p99   float64
}

func (r rankedCall) consider(c rankedCall, delta histogram) rankedCall {
	if delta.count < minResourceCalls {
		return r
	}
	p99, ok := delta.quantile(0.99)
	if ok && p99 > r.p99 {
		c.p99 = p99
		return c
	}
	return r
}

func summarize(all histogram, worst rankedCall) latency {
	p99, ok := all.quantile(0.99)
	if !ok || all.count < minClassCalls {
		return latency{}
	}
	return latency{p99: p99, calls: all.count, slowest: worst.label}
}

func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// addControlPlaneMetrics records what one /metrics response says about
// the API server's own health on attrs, and returns the observations
// that belong to other entities (the admission webhooks).
func (p *Prober) addControlPlaneMetrics(
	body []byte, attrs map[string]inventory.Value, now time.Time,
) []inventory.Observation {
	reading := scanControlPlane(body)
	addStorageAttributes(reading, attrs)
	previous, ok := p.plane.compare(reading, now)
	if !ok {
		return nil
	}
	addLatencyAttributes(reading, previous.reading, attrs)
	addThrottleAttributes(reading, previous, now, attrs)
	return p.webhookObservations(reading, previous.reading, now)
}

// addLatencyAttributes records the percentiles of the calls made since
// the previous reading.
func addLatencyAttributes(
	now, before *planeReading, attrs map[string]inventory.Value,
) {
	writes, reads := classLatencies(now, before)
	setLatency(attrs, writes, AttrWritesP99, AttrWritesCalls,
		AttrWritesSlowest)
	setLatency(attrs, reads, AttrReadsP99, AttrReadsCalls,
		AttrReadsSlowest)
	if delta, ok := deltaOf(now.etcd.get("etcd"), before.etcd.get("etcd")); ok {
		if p99, ok := delta.quantile(0.99); ok &&
			delta.count >= minClassCalls {
			attrs[AttrEtcdP99] = inventory.Number(p99 * 1000)
		}
	}
}

func setLatency(
	attrs map[string]inventory.Value, l latency, p99, calls, slowest string,
) {
	if l.calls == 0 {
		return
	}
	attrs[p99] = inventory.Number(l.p99 * 1000)
	attrs[calls] = inventory.Number(l.calls)
	if l.slowest != "" {
		attrs[slowest] = inventory.Text(l.slowest)
	}
}

// addThrottleAttributes records requests that priority and fairness
// rejected since the previous reading, and the queue now.
func addThrottleAttributes(
	now *planeReading, before planeSample, at time.Time,
	attrs map[string]inventory.Value,
) {
	if seconds := at.Sub(before.at).Seconds(); seconds > 0 {
		if total, level, ok := rejectedSince(now, before.reading); ok {
			attrs[AttrThrottledRate] = inventory.Number(total / seconds)
			if level != "" {
				attrs[AttrThrottledLevel] = inventory.Text(level)
			}
		}
	}
	queued := 0.0
	for _, n := range now.inqueue {
		queued += n
	}
	attrs[AttrQueuedRequests] = inventory.Number(queued)
}

// rejectedSince counts the requests rejected between two readings and
// names the priority level that rejected the most. It is false when a
// counter went backwards.
func rejectedSince(
	now, before *planeReading,
) (total float64, level string, ok bool) {
	worst := 0.0
	for _, key := range sortedNames(now.counters) {
		if !strings.HasPrefix(key, "apf/") {
			continue
		}
		rejected := now.counters[key] - before.counters[key]
		if rejected < 0 {
			return 0, "", false
		}
		total += rejected
		if rejected > worst {
			worst, level = rejected, strings.TrimPrefix(key, "apf/")
		}
	}
	return total, level, true
}

// addStorageAttributes records the etcd database size and the resource
// with the most stored objects. Both are gauges, so one reading is
// enough.
func addStorageAttributes(
	reading *planeReading, attrs map[string]inventory.Value,
) {
	if reading.dbBytes > 0 {
		attrs[AttrEtcdDBBytes] = inventory.Number(reading.dbBytes)
	}
	top, count := "", 0.0
	for _, resource := range sortedNames(reading.objects) {
		if n := reading.objects[resource]; n > count {
			top, count = resource, n
		}
	}
	if top != "" {
		attrs[AttrObjectsResource] = inventory.Text(top)
		attrs[AttrObjectsCount] = inventory.Number(count)
	}
}
