package detectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Limits for the API server's own load, read from its metrics. They are
// fixed: the writes target is the Kubernetes one (99% of mutating calls
// under one second), the others are far enough past normal that a
// healthy cluster never reaches them.
const (
	slowWritesMS    = 1000.0
	extremeWritesMS = 5000.0
	slowReadsMS     = 5000.0
	extremeReadsMS  = 15000.0
	// throttledPerSecond is requests rejected per second that a
	// busy but healthy cluster does not see for minutes.
	throttledPerSecond = 1.0
	// etcdQuotaBytes is etcd's default size limit; a managed cluster may
	// allow more, so only the approach to it is reported, as information.
	etcdQuotaBytes = 2 << 30
	etcdNearShare  = 0.8
	// manyObjects is one resource's stored objects at which listing it
	// is a heavy call.
	manyObjects = 100000.0
	// holdShare is the fraction of a limit a value must stay above for a
	// finding already raised to continue, so a value hovering at the
	// limit does not raise and clear on every round.
	holdShare = 0.7
	// slowSustain is how long latency must stay over its limit.
	// Extreme latency, which gets a warning, needs only extremeSustain.
	slowSustain    = 5 * time.Minute
	extremeSustain = 2 * time.Minute
	// throttleSustain is how long throttling must continue.
	throttleSustain = 3 * time.Minute
)

// APIServerLoad detects an API server that is slow, throttling or
// holding too much, from its own /metrics. It complements
// ClusterService, which only sees whether the probe request answered.
// Every finding is sustained for minutes before it is raised: the
// numbers cover the last 30 seconds only.
type APIServerLoad struct{}

// Name implements detection.Detector.
func (APIServerLoad) Name() string { return "apiserver-load" }

// Kinds implements detection.Detector.
func (APIServerLoad) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.APIServer.Kind}
}

// Detect implements detection.Detector.
func (APIServerLoad) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if e.ID != kube.APIServer {
		return nil
	}
	var out []detection.Finding
	out = append(out, slowCalls(ctx, e, writeLimits)...)
	out = append(out, slowCalls(ctx, e, readLimits)...)
	out = append(out, throttling(ctx, e)...)
	out = append(out, etcdSize(ctx, e)...)
	return append(out, manyStored(ctx, e)...)
}

// held reports whether value is over limit, or still clearly over it
// after the finding named key was raised.
func held(ctx detection.Context, key string, value, limit float64) bool {
	return value >= limit ||
		(ctx.Ongoing(key) && value >= holdShare*limit)
}

// callLimits describe one class of calls to judge.
type callLimits struct {
	key, noun, reason    string
	p99Attr, slowestAttr string
	slowMS, extremeMS    float64
	callsAttr            string
}

var writeLimits = callLimits{
	key: "api-writes-slow", noun: "writes",
	reason:  reasons.APIServerWritesSlow,
	p99Attr: kube.AttrWritesP99, callsAttr: kube.AttrWritesCalls,
	slowestAttr: kube.AttrWritesSlowest,
	slowMS:      slowWritesMS, extremeMS: extremeWritesMS,
}

var readLimits = callLimits{
	key: "api-reads-slow", noun: "reads",
	reason:  reasons.APIServerReadsSlow,
	p99Attr: kube.AttrReadsP99, callsAttr: kube.AttrReadsCalls,
	slowestAttr: kube.AttrReadsSlowest,
	slowMS:      slowReadsMS, extremeMS: extremeReadsMS,
}

// slowCalls reports calls whose 99th percentile stayed over the limit.
// It is information, a line in the digest, unless it is extreme.
func slowCalls(
	ctx detection.Context, e inventory.Entity, l callLimits,
) []detection.Finding {
	p99, ok := number(e, l.p99Attr)
	if !ok || !held(ctx, l.key, p99, l.slowMS) {
		return nil
	}
	severity, wait := detection.Info, slowSustain
	if p99 >= l.extremeMS {
		severity, wait = detection.Warning, extremeSustain
	}
	since := ctx.Onset(l.key, ctx.Now)
	if !sustained(ctx, l.key, since, wait) {
		return nil
	}
	calls, _ := number(e, l.callsAttr)
	return []detection.Finding{{
		Reason: l.reason, Severity: severity, Since: since,
		Summary: "API server " + l.noun + " are slow: p99 " +
			millisText(p99) + ", target " + millisText(l.slowMS) +
			slowestText(ctx, e, l),
		Evidence: []detection.Evidence{
			{Label: "p99 per call", Value: millisText(p99)},
			{Label: "calls measured", Value: fmt.Sprintf("%.0f", calls)},
			{Label: "etcd p99 per call", Value: etcdText(e)},
		},
	}}
}

// slowestText says what the slow calls were: the verb and resource, and
// the admission webhook that is slow too, if one is.
func slowestText(
	ctx detection.Context, e inventory.Entity, l callLimits,
) string {
	out := ""
	if slowest := strings.TrimSpace(text(e, l.slowestAttr)); slowest != "" {
		out = "; slowest: " + slowest
	}
	hook := slowWebhook(ctx)
	switch {
	case hook == "" || l.noun != "writes":
		return out
	case out == "":
		return "; a webhook is slow: " + hook
	}
	return out + " via webhook " + hook
}

// etcdText is the API server's own p99 of calls to etcd.
func etcdText(e inventory.Entity) string {
	if ms, ok := number(e, kube.AttrEtcdP99); ok {
		return millisText(ms)
	}
	return "unknown"
}

// slowWebhook is the name of the webhook that is slowest now, if one is
// slow enough to slow every write that passes it.
func slowWebhook(ctx detection.Context) string {
	if ctx.Model == nil {
		return ""
	}
	worst, name := slowWritesMS, ""
	for _, kind := range []inventory.Kind{
		kube.KindMutatingWebhook, kube.KindValidatingHook,
	} {
		for _, id := range ctx.Model.Entities(kind) {
			hook, ok := ctx.Model.Entity(id)
			if !ok {
				continue
			}
			if p99, ok := number(hook, kube.AttrWebhookP99); ok &&
				p99 >= worst {
				worst, name = p99, text(hook, kube.AttrWebhookSlowest)
			}
		}
	}
	return name
}

// throttling reports requests that priority and fairness refuse for
// minutes: clients get 429 and retry, so controllers fall behind.
func throttling(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	rate, ok := number(e, kube.AttrThrottledRate)
	if !ok || !held(ctx, "api-throttling", rate, throttledPerSecond) {
		return nil
	}
	since := ctx.Onset("api-throttling", ctx.Now)
	if !sustained(ctx, "api-throttling", since, throttleSustain) {
		return nil
	}
	level := text(e, kube.AttrThrottledLevel)
	queued, _ := number(e, kube.AttrQueuedRequests)
	return []detection.Finding{{
		Reason: reasons.APIServerThrottling, Severity: detection.Warning,
		Since: since,
		Summary: fmt.Sprintf("API server is rejecting %.1f requests a "+
			"second (priority level %s)", rate, level),
		Evidence: []detection.Evidence{
			{Label: "priority level", Value: level},
			{Label: "requests waiting", Value: fmt.Sprintf("%.0f", queued)},
		},
	}}
}
