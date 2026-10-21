package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/safego"
)

// sourceHealthInterval is how often source availability is re-published
// after the initial sync, so a kind that later syncs clears its status.
const sourceHealthInterval = 30 * time.Second

// sourceStatusSink is the health server as source reporting sees it.
type sourceStatusSink interface {
	SetComponentStatus(name, state, reason string, available bool)
}

// sourceAvailability is the Kubernetes source as reporting sees it.
type sourceAvailability interface {
	Unavailable() []kube.SourceStatus
}

// sourceHealth publishes each unsynced watched resource as a health
// component named "source-<resource>", with a bounded reason. Component
// names come from the fixed informer registrations, so they are bounded.
type sourceHealth struct {
	sink     sourceStatusSink
	source   sourceAvailability
	reported map[string]string
	coverage coveragePublisher
	note     coverageNote
	ready    func(bool)
	// panics reads the recovered handler panics; nil skips the check.
	panics func() []kubeclient.HandlerPanicCount
	// panicSeen is each resource's panic count at the previous report.
	panicSeen map[string]int64
	// syncing is set while the sources are still listing: reports then
	// leave readiness alone (see run). Only run's goroutine touches it.
	syncing bool
}

func newSourceHealth(
	sink sourceStatusSink, source sourceAvailability,
) *sourceHealth {
	return &sourceHealth{
		sink: sink, source: source, reported: map[string]string{},
	}
}

// withCoverage also publishes the watch coverage summary on every report.
// It is applied once before the first report.
func (h *sourceHealth) withCoverage(p coveragePublisher) *sourceHealth {
	h.coverage = p
	return h
}

// withHandlerPanics also degrades a resource whose event handler panicked
// since the previous report. It is applied once before the first report.
func (h *sourceHealth) withHandlerPanics(
	read func() []kubeclient.HandlerPanicCount,
) *sourceHealth {
	h.panics = read
	h.panicSeen = map[string]int64{}
	return h
}

// panicking lists the inventory resources whose handler panicked since
// the previous call. The state clears one report after the panics stop.
func (h *sourceHealth) panicking() []kube.SourceStatus {
	if h.panics == nil {
		return nil
	}
	var out []kube.SourceStatus
	for _, p := range h.panics() {
		if p.Component != "inventory" {
			continue
		}
		if p.Count > h.panicSeen[p.Resource] {
			out = append(out, kube.SourceStatus{
				Resource: p.Resource, Reason: kube.ReasonHandlerPanics,
			})
		}
		h.panicSeen[p.Resource] = p.Count
	}
	return out
}

// withReadiness also reports whether every required resource is available
// on each report. It is applied once before the first report.
func (h *sourceHealth) withReadiness(set func(bool)) *sourceHealth {
	h.ready = set
	return h
}

// requiredAvailable reports whether no required resource is unavailable.
func requiredAvailable(statuses []kube.SourceStatus) bool {
	for _, status := range statuses {
		if status.Required {
			return false
		}
	}
	return true
}

// staleSources is a source that can also tell which synced resources have
// a watch that keeps failing (kube.Source.Stale).
type staleSources interface {
	Stale() []kube.SourceStatus
}

// run re-publishes availability on every tick until ctx ends. Until
// synced closes, reports only publish what is unavailable: readiness
// stays with the startup hold, which ends when the sources finished
// their initial list. The report made as synced closes sets it.
func (h *sourceHealth) run(
	ctx context.Context, tick <-chan time.Time, synced <-chan struct{},
) {
	h.syncing = synced != nil
	for {
		select {
		case <-ctx.Done():
			return
		case <-synced:
			synced, h.syncing = nil, false
			h.safeReport()
		case <-tick:
			h.safeReport()
		}
	}
}

// safeReport is report with a panic contained: a failed report is made
// again at the next tick.
func (h *sourceHealth) safeReport() {
	safego.Do("source-health", h.report)
}

// report publishes transitions only: a newly unavailable resource is
// degraded (and counted once), a recovered one returns to running.
func (h *sourceHealth) report() {
	if summary, ok := h.coverage.publish(); ok {
		h.note.observe(h.sink, summary)
	}
	if h.source == nil {
		return
	}
	statuses := h.source.Unavailable()
	if h.ready != nil && !h.syncing {
		h.ready(requiredAvailable(statuses))
	}
	if h.sink != nil {
		shown := withUnlisted(statuses, h.stale())
		h.publish(withUnlisted(shown, h.panicking()))
	}
}

// stale lists the synced resources whose watch keeps failing. They stay
// available (readiness does not change); health only shows them.
func (h *sourceHealth) stale() []kube.SourceStatus {
	if source, ok := h.source.(staleSources); ok {
		return source.Stale()
	}
	return nil
}

// withUnlisted adds the extra resources that are not already listed as
// unavailable for a more basic reason.
func withUnlisted(
	statuses, extra []kube.SourceStatus,
) []kube.SourceStatus {
	for _, e := range extra {
		listed := false
		for _, s := range statuses {
			listed = listed || (s.Resource == e.Resource && s.Group == e.Group)
		}
		if !listed {
			statuses = append(statuses, e)
		}
	}
	return statuses
}

func (h *sourceHealth) publish(statuses []kube.SourceStatus) {
	current := map[string]string{}
	for _, status := range statuses {
		name := "source-" + status.Resource
		if status.Group != "" {
			name += "." + status.Group
		}
		reason := sourceHealthReason(status)
		current[name] = reason
		if h.reported[name] == reason {
			continue
		}
		if _, was := h.reported[name]; !was {
			metrics.DefaultRegistry().SourceUnavailable.Add(1)
		}
		klog.InfoS("watched resource unavailable", "component", "pipeline",
			"operation", "source_sync", "resource", status.Resource,
			"group", status.Group, "required", status.Required,
			"reason", reason)
		h.sink.SetComponentStatus(name, "degraded", reason, false)
	}
	for name := range h.reported {
		if _, still := current[name]; !still {
			h.sink.SetComponentStatus(name, "running", "", true)
		}
	}
	h.reported = current
}

// sourceHealthReason maps source reason codes onto the health vocabulary;
// optional kinds use the optional_* codes so they read as degradation.
func sourceHealthReason(status kube.SourceStatus) string {
	switch status.Reason {
	case kube.ReasonPermissionDenied:
		if status.Required {
			return "permission_denied"
		}
		return "optional_permission_denied"
	case kube.ReasonAPIUnavailable:
		if status.Required {
			return "api_unavailable"
		}
		return "optional_api_unavailable"
	case kube.ReasonSyncTimeout:
		return "cache_sync_timeout"
	case kube.ReasonSyncPending:
		return "cache_sync_pending"
	case kube.ReasonHandlerPanics:
		return "handler_panics"
	default:
		return "cache_sync_failed"
	}
}
