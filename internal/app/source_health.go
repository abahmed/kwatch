package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
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
	ready    func(bool)
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

// run re-publishes availability on every tick until ctx ends.
func (h *sourceHealth) run(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			h.report()
		}
	}
}

// report publishes transitions only: a newly unavailable resource is
// degraded (and counted once), a recovered one returns to running.
func (h *sourceHealth) report() {
	h.coverage.publish()
	if h.source == nil {
		return
	}
	statuses := h.source.Unavailable()
	if h.ready != nil {
		h.ready(requiredAvailable(statuses))
	}
	if h.sink != nil {
		h.publish(statuses)
	}
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
	default:
		return "cache_sync_failed"
	}
}
